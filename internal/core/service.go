package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Service is the OmaVM Core: the single entry point the CLI and, later,
// the GUI call into. It owns environment lifecycle and persistence; it
// never runs backend-specific commands itself, only through Backend.
type Service struct {
	store    Store
	backends map[EnvironmentKind]Backend
	launcher Launcher
}

// NewService wires a Store with the backends responsible for each Kind.
// A nil backend for a given Kind is valid: operations on that Kind will
// fail with ErrUnsupported instead of panicking.
func NewService(store Store, box, machine Backend) *Service {
	return &Service{
		store: store,
		backends: map[EnvironmentKind]Backend{
			Box:     box,
			Machine: machine,
		},
	}
}

// SetLauncher enables publishing environments in the host launcher. A
// Service without one (tests, a host without a home directory) simply
// doesn't publish.
func (s *Service) SetLauncher(l Launcher) {
	s.launcher = l
}

// syncLauncher brings the environment's launcher entry in line with its
// settings, best-effort: a read-only home or a full disk must not fail
// the lifecycle operation that triggered it.
func (s *Service) syncLauncher(env Environment) {
	if s.launcher == nil {
		return
	}
	var err error
	if env.Settings.LauncherDisabled {
		err = s.launcher.Withdraw(env)
	} else {
		err = s.launcher.Publish(env)
	}
	if err != nil {
		slog.Warn("launcher entry not updated", "environment", env.Name, "error", err)
	}
}

func (s *Service) backendFor(kind EnvironmentKind) (Backend, error) {
	b := s.backends[kind]
	if b == nil {
		return nil, Unsupportedf("no backend registered for kind %s", kind)
	}
	return b, nil
}

// validateEnvironmentName rejects names that would be unsafe once a
// Backend turns them into part of a filesystem path. Machines use Name
// directly as a raw path component (internal/backend/qemu's dir()); a
// name like ".." or containing "/" would let a state directory escape
// its parent instead of just failing to boot. Boxes derive their own
// sanitized container name from Name and never touch a path with the
// raw value, but the same rule keeps one name meaning across both
// kinds instead of a Machine-only special case. Otherwise intentionally
// permissive: spaces, accents and most punctuation are fine.
func validateEnvironmentName(name string) error {
	if name == "" {
		return Invalidf("environment name is required")
	}
	if name == "." || name == ".." {
		return Invalidf("%q is not a valid environment name", name)
	}
	// Commands read a leading dash as an option: a Machine called
	// "--json" could not be reached by `omavm status --json`.
	if strings.HasPrefix(name, "-") {
		return Invalidf("environment name cannot start with %q", "-")
	}
	for _, r := range name {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return Invalidf("environment name cannot contain %q", string(r))
		}
	}
	return nil
}

func findEnvironment(envs []Environment, name string) (Environment, int) {
	for i, e := range envs {
		if e.Name == name {
			return e, i
		}
	}
	return Environment{}, -1
}

// List returns every known environment, regardless of Kind or Backend.
func (s *Service) List(ctx context.Context) ([]Environment, error) {
	return s.store.Load()
}

// Create registers a new environment and delegates its creation to the
// Backend responsible for its Kind.
func (s *Service) Create(ctx context.Context, env Environment) (Environment, error) {
	env.Name = strings.TrimSpace(env.Name)
	if err := validateEnvironmentName(env.Name); err != nil {
		return Environment{}, err
	}
	// Boxes turn Name into a sanitized container name (internal/backend/
	// distrobox's boxName) and never touch the filesystem with it
	// directly. Machines use it as a raw path component
	// (internal/backend/qemu's dir()), so validateEnvironmentName above
	// is what actually keeps a Machine's state directory from escaping
	// its parent — this Image check is a separate, later failure a user
	// would otherwise only see after the (slower) backend.Create call.
	// A Machine starts with an empty disk: without installation media it
	// has nothing to boot, and nothing can attach an ISO later.
	// Without one, `distrobox create --image ""` never returns.
	if env.Kind == Box && strings.TrimSpace(env.Image) == "" {
		return Environment{}, Invalidf("a Box needs a container image to start from (--image fedora:latest, ubuntu:24.04, archlinux, ...)")
	}
	if env.Kind == Machine && env.Image == "" {
		return Environment{}, Invalidf("a Machine needs an installation ISO to boot from (--image path/to/system.iso)")
	}
	if env.Kind == Machine {
		info, statErr := os.Stat(env.Image)
		if statErr != nil {
			return Environment{}, Invalidf("installation media %q: %v", env.Image, statErr)
		}
		if info.IsDir() {
			return Environment{}, Invalidf("installation media %q is a directory, not an ISO file", env.Image)
		}
	}
	// Same bounds Configure enforces on an existing Machine (below), kept
	// in sync so create-time hardware and later Settings edits mean the
	// same thing instead of drifting into two silently different rules.
	if env.Settings.CPUs != 0 || env.Settings.MemoryMiB != 0 {
		if env.Kind != Machine {
			return Environment{}, Unsupportedf("CPU and memory settings only apply to Machines")
		}
		if env.Settings.CPUs != 0 && (env.Settings.CPUs < 1 || env.Settings.CPUs > 64) {
			return Environment{}, Invalidf("cpus must be between 1 and 64")
		}
		if env.Settings.MemoryMiB != 0 && (env.Settings.MemoryMiB < 256 || env.Settings.MemoryMiB > 262144) {
			return Environment{}, Invalidf("memory-mib must be between 256 and 262144")
		}
	}

	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return Environment{}, err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return Environment{}, err
	}
	if _, idx := findEnvironment(envs, env.Name); idx != -1 {
		return Environment{}, fmt.Errorf("%w: %s", ErrAlreadyExists, env.Name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return Environment{}, err
	}

	id, err := newID()
	if err != nil {
		return Environment{}, err
	}
	env.ID = id

	if err := backend.Create(ctx, env); err != nil {
		return Environment{}, fmt.Errorf("create %s: %w", env.Name, err)
	}
	// Persist ownership only after the backend completed creation.
	env.Backend = backend.Name()

	envs = append(envs, env)
	if err := s.store.Save(envs); err != nil {
		// The backend already created a real resource (a container, a
		// VM disk) but the registry write failed — this would otherwise
		// leave that resource orphaned: it exists, but nothing in the
		// CLI/GUI can see or manage it since it was never registered.
		// Best-effort undo the backend side too, so a failure here
		// reads the same as the create having never happened, instead
		// of silently leaking a resource (same "never leave a
		// half-succeeded operation invisible" rule as the snapshot
		// retention fix above).
		if removeErr := backend.Remove(ctx, env); removeErr != nil {
			return Environment{}, fmt.Errorf("save registry after creating %s: %w (cleanup also failed, resource may be orphaned: %v)", env.Name, err, removeErr)
		}
		return Environment{}, fmt.Errorf("save registry after creating %s: %w", env.Name, err)
	}
	s.syncLauncher(env)
	return env, nil
}

func (s *Service) resolve(ctx context.Context, name string) (Environment, Backend, error) {
	envs, err := s.store.Load()
	if err != nil {
		return Environment{}, nil, err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return Environment{}, nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return Environment{}, nil, err
	}
	return env, backend, nil
}

func (s *Service) Start(ctx context.Context, name string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if err := backend.Start(ctx, env); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	// Also publishes environments created before launcher entries
	// existed, the first time they are used.
	s.syncLauncher(env)
	return nil
}

func (s *Service) Open(ctx context.Context, name string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if err := backend.Open(ctx, env); err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	s.syncLauncher(env)
	return nil
}

// Preview returns a screenshot path for environments whose Backend
// implements Previewer (currently only Machines). ErrUnsupported for
// anything else — a Box has no display, and that's never hidden behind
// a placeholder image pretending otherwise.
func (s *Service) Preview(ctx context.Context, name string) (string, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return "", err
	}
	previewer, ok := backend.(Previewer)
	if !ok {
		return "", Unsupportedf("%s has no preview capability", name)
	}
	path, err := previewer.Preview(ctx, env)
	if err != nil {
		return "", fmt.Errorf("preview %s: %w", name, err)
	}
	return path, nil
}

func (s *Service) Stop(ctx context.Context, name string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if err := backend.Stop(ctx, env); err != nil {
		return fmt.Errorf("stop %s: %w", name, err)
	}
	return nil
}

func (s *Service) advancedLifecycle(ctx context.Context, name string) (Environment, AdvancedLifecycle, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return Environment{}, nil, err
	}
	lifecycle, ok := backend.(AdvancedLifecycle)
	if !ok {
		return Environment{}, nil, Unsupportedf("%s is a Box: restart, pause, resume and force stop are only available for Machines", name)
	}
	return env, lifecycle, nil
}

func (s *Service) Restart(ctx context.Context, name string) error {
	env, lifecycle, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	return lifecycle.Restart(ctx, env)
}

func (s *Service) Pause(ctx context.Context, name string) error {
	env, lifecycle, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	return lifecycle.Pause(ctx, env)
}

func (s *Service) Resume(ctx context.Context, name string) error {
	env, lifecycle, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	return lifecycle.Resume(ctx, env)
}

func (s *Service) ForceStop(ctx context.Context, name string) error {
	env, lifecycle, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	return lifecycle.ForceStop(ctx, env)
}

func (s *Service) Status(ctx context.Context, name string) (Status, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return Status{}, err
	}
	status, err := backend.Status(ctx, env)
	if err != nil {
		return Status{}, fmt.Errorf("status %s: %w", name, err)
	}
	return status, nil
}

func (s *Service) Integration(ctx context.Context, name string) (IntegrationReport, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return IntegrationReport{}, err
	}
	reporter, ok := backend.(IntegrationReporter)
	if !ok {
		return IntegrationReport{}, Unsupportedf("%s is a Box: guest tools are only checked for Machines", name)
	}
	return reporter.Integration(ctx, env)
}

// InspectHost reports host capabilities from every backend that offers
// them, Machine first.
func (s *Service) InspectHost(ctx context.Context) ([]HostCapability, error) {
	var all []HostCapability
	for _, kind := range []EnvironmentKind{Machine, Box} {
		inspector, ok := s.backends[kind].(HostInspector)
		if !ok {
			continue
		}
		caps, err := inspector.InspectHost(ctx)
		if err != nil {
			return nil, fmt.Errorf("inspect host: %w", err)
		}
		all = append(all, caps...)
	}
	return all, nil
}

func (s *Service) Settings(ctx context.Context, name string) (EnvironmentSettings, error) {
	env, _, err := s.resolve(ctx, name)
	if err != nil {
		return EnvironmentSettings{}, err
	}
	return env.EffectiveSettings(), nil
}

func (s *Service) Configure(ctx context.Context, name string, patch SettingsPatch) (EnvironmentSettings, error) {
	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return EnvironmentSettings{}, err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return EnvironmentSettings{}, err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return EnvironmentSettings{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	// Start from the raw, persisted settings — not EffectiveSettings().
	// EffectiveSettings() substitutes defaults for CPUs/MemoryMiB/
	// SnapshotLimit (0 -> 2/2048/10) purely for display; starting the
	// patch from that resolved copy would silently re-persist those
	// defaults as if explicitly pinned on every single Configure call,
	// even ones that only touch an unrelated field like Description.
	// That's a real bug this project hit: it permanently disables Travel
	// Mode's automatic CPU reduction (internal/backend/qemu/qemu.go's
	// Start only reduces CPUs when the raw setting is still 0) the first
	// time anyone saves Settings for any reason (docs/TODO.md P2
	// "salvar uma descrição não altera inadvertidamente a intenção de
	// configuração de CPU"). Only patch.CPUs/MemoryMiB/SnapshotLimit
	// being explicitly set below should ever turn a 0 into a pinned
	// value.
	settings := env.Settings
	if patch.DisconnectISO != nil {
		if env.Kind != Machine {
			return EnvironmentSettings{}, Unsupportedf("installation media only applies to Machines")
		}
		settings.DisconnectISO = *patch.DisconnectISO
	}
	if patch.Description != nil {
		settings.Description = strings.TrimSpace(*patch.Description)
		if len(settings.Description) > 500 {
			return EnvironmentSettings{}, Invalidf("description must be at most 500 characters")
		}
	}
	if patch.CPUs != nil || patch.MemoryMiB != nil {
		if env.Kind != Machine {
			return EnvironmentSettings{}, Unsupportedf("CPU and memory settings only apply to Machines")
		}
	}
	if patch.SharedPath != nil || patch.SharedReadOnly != nil {
		if env.Kind != Machine {
			return EnvironmentSettings{}, Unsupportedf("shared folders only apply to Machines")
		}
	}
	if patch.SharedPath != nil {
		path := strings.TrimSpace(*patch.SharedPath)
		if path != "" {
			path, err = filepath.Abs(path)
			if err != nil {
				return EnvironmentSettings{}, fmt.Errorf("resolve shared folder: %w", err)
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				return EnvironmentSettings{}, fmt.Errorf("shared folder: %w", statErr)
			}
			if !info.IsDir() {
				return EnvironmentSettings{}, Invalidf("shared folder must be a directory")
			}
		}
		settings.SharedPath = path
	}
	if patch.SharedReadOnly != nil {
		settings.SharedReadOnly = *patch.SharedReadOnly
	}
	if patch.CPUs != nil {
		if *patch.CPUs < 1 || *patch.CPUs > 64 {
			return EnvironmentSettings{}, Invalidf("cpus must be between 1 and 64")
		}
		settings.CPUs = *patch.CPUs
	}
	if patch.MemoryMiB != nil {
		if *patch.MemoryMiB < 256 || *patch.MemoryMiB > 262144 {
			return EnvironmentSettings{}, Invalidf("memory-mib must be between 256 and 262144")
		}
		settings.MemoryMiB = *patch.MemoryMiB
	}
	if patch.SnapshotLimit != nil {
		if *patch.SnapshotLimit < 1 || *patch.SnapshotLimit > 100 {
			return EnvironmentSettings{}, Invalidf("snapshot-limit must be between 1 and 100")
		}
		settings.SnapshotLimit = *patch.SnapshotLimit
	}
	if patch.Color != nil {
		color := strings.TrimSpace(*patch.Color)
		if color != "" && !validColor(color) {
			return EnvironmentSettings{}, Invalidf("color must be one of: %s", strings.Join(EnvironmentColors, ", "))
		}
		settings.Color = color
	}
	if patch.ShareClipboard != nil {
		// A Machine shares the clipboard with the guest; a Box lets
		// programs in its terminal copy to the host clipboard (OSC 52).
		settings.ClipboardDisabled = !*patch.ShareClipboard
	}
	if patch.TravelMode != nil {
		// Both Kinds: a Machine starts with fewer CPUs, a Box's container
		// is limited in place.
		settings.TravelModeDisabled = !*patch.TravelMode
	}
	if patch.Vulkan != nil {
		if env.Kind != Machine {
			return EnvironmentSettings{}, Unsupportedf("Vulkan acceleration only applies to Machines")
		}
		settings.VulkanDisabled = !*patch.Vulkan
	}
	if patch.SSH != nil {
		if env.Kind != Machine {
			return EnvironmentSettings{}, Unsupportedf("SSH only applies to Machines; a Box opens with omavm open")
		}
		settings.SSHDisabled = !*patch.SSH
	}
	if patch.Launcher != nil {
		settings.LauncherDisabled = !*patch.Launcher
	}
	if patch.OpenInEmptyWorkspace != nil {
		// Unlike CPU/memory/shared folder/clipboard/travel mode, this
		// applies to both Kinds: a Box's terminal opens through the same
		// desktop-integration path as a Machine's viewer (docs/TODO.md P2).
		settings.EmptyWorkspaceDisabled = !*patch.OpenInEmptyWorkspace
	}
	env.Settings = settings
	envs[idx] = env
	if err := s.store.Save(envs); err != nil {
		return EnvironmentSettings{}, err
	}
	// Name, color and description all show in the entry.
	s.syncLauncher(env)

	// HostLinker is an optional capability (only Machines today): giving
	// the environment a host-visible tagged path is a best-effort bonus
	// on top of the color tag, never a reason to fail Configure itself.
	if patch.Color != nil {
		if backend, backendErr := s.backendFor(env.Kind); backendErr == nil {
			if linker, ok := backend.(HostLinker); ok {
				_, _ = linker.Link(ctx, env, settings.Color)
			}
		}
	}
	// The caller (CLI/GUI) still wants the resolved, display-ready
	// values (e.g. "2 CPUs" instead of "0"), just never persisted as a
	// pin unless actually patched above.
	return env.EffectiveSettings(), nil
}

// snapshotTag derives the technical tag a Backend addresses a snapshot
// by from the user's Label, plus a short random suffix so two snapshots
// with the same Label never collide. The Label, not this tag, is what
// CLI/GUI show a human (UX Principle #9).
func snapshotTag(label string) (string, error) {
	var sanitized strings.Builder
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			sanitized.WriteRune(r)
		case sanitized.Len() > 0 && !strings.HasSuffix(sanitized.String(), "-"):
			sanitized.WriteByte('-')
		}
	}
	clean := strings.Trim(sanitized.String(), "-")
	if clean == "" {
		clean = "snapshot"
	}
	suffix, err := newID()
	if err != nil {
		return "", err
	}
	return clean + "-" + suffix[:8], nil
}

// CreateSnapshot captures the environment's current state under a
// significant Label, delegating the actual capture to the Backend's
// SnapshotManager capability and enforcing SnapshotLimit by discarding
// the oldest snapshot first (UX Principle #9).
func (s *Service) CreateSnapshot(ctx context.Context, name, label string) (Snapshot, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return Snapshot{}, Invalidf("snapshot label is required")
	}

	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return Snapshot{}, err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return Snapshot{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return Snapshot{}, err
	}
	manager, ok := backend.(SnapshotManager)
	if !ok {
		return Snapshot{}, Unsupportedf("%s is a Box: snapshots are only available for Machines for now", name)
	}

	tag, err := snapshotTag(label)
	if err != nil {
		return Snapshot{}, err
	}
	if err := manager.CreateSnapshot(ctx, env, tag); err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot %s: %w", name, err)
	}
	snap := Snapshot{ID: tag, Label: label, CreatedAt: time.Now().UTC()}
	env.Snapshots = append(env.Snapshots, snap)

	// Persist right after the backend confirms the new snapshot, before
	// touching retention: the qcow2/QMP snapshot already exists at this
	// point, so if a later discard below fails partway, the registry
	// must not lose track of it — that would leave a snapshot the
	// backend has but the Core doesn't know about, invisible to
	// `snapshot list` with no way to reach it again except by editing
	// the state file by hand.
	envs[idx] = env
	if err := s.store.Save(envs); err != nil {
		return Snapshot{}, err
	}

	limit := env.EffectiveSettings().SnapshotLimit
	for limit > 0 && len(env.Snapshots) > limit {
		oldest := env.Snapshots[0]
		if err := manager.RemoveSnapshot(ctx, env, oldest.ID); err != nil {
			// snap itself was already saved above and is not lost; only
			// the retention discard failed, so surface that distinctly
			// while still returning the snapshot that really was created.
			return snap, fmt.Errorf("created %s, but discarding the oldest snapshot for %s failed: %w", label, name, err)
		}
		env.Snapshots = env.Snapshots[1:]
		// Persist each discard immediately too: the backend has already
		// deleted this one, so the registry must not still list it if a
		// later discard in this same loop fails.
		envs[idx] = env
		if err := s.store.Save(envs); err != nil {
			return snap, err
		}
	}

	return snap, nil
}

// ListSnapshots only reads the store: the Core, not the Backend, owns
// snapshot history and labels.
func (s *Service) ListSnapshots(ctx context.Context, name string) ([]Snapshot, error) {
	env, _, err := s.resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	return env.Snapshots, nil
}

func findSnapshot(snapshots []Snapshot, id string) int {
	for i, snap := range snapshots {
		if snap.ID == id {
			return i
		}
	}
	return -1
}

// GoToSnapshot restores the environment to a prior captured state. Named
// "Go To" rather than "revert"/"restore" per UX Principle #9.
//
// Takes the same registry lock CreateSnapshot/RemoveSnapshot hold across
// their whole backend call, even though this doesn't itself mutate the
// registry: reproduced 2026-09-27 (docs/TODO.md P0) that a concurrent
// CreateSnapshot/RemoveSnapshot racing a lock-free GoToSnapshot on the
// same stopped Machine hits qemu-img's own image locking ("Failed to get
// \"write\" lock") — no corruption, but a confusing raw error for
// whichever call lost. Serializing here removes the race instead of
// prettifying the error.
func (s *Service) GoToSnapshot(ctx context.Context, name, id string) error {
	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return err
	}
	manager, ok := backend.(SnapshotManager)
	if !ok {
		return Unsupportedf("%s is a Box: snapshots are only available for Machines for now", name)
	}
	if findSnapshot(env.Snapshots, id) == -1 {
		return fmt.Errorf("%w: snapshot %s", ErrNotFound, id)
	}
	if err := manager.GoToSnapshot(ctx, env, id); err != nil {
		return fmt.Errorf("go to snapshot on %s: %w", name, err)
	}
	return nil
}

func (s *Service) RemoveSnapshot(ctx context.Context, name, id string) error {
	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return err
	}
	manager, ok := backend.(SnapshotManager)
	if !ok {
		return Unsupportedf("%s is a Box: snapshots are only available for Machines for now", name)
	}
	snapIdx := findSnapshot(env.Snapshots, id)
	if snapIdx == -1 {
		return fmt.Errorf("%w: snapshot %s", ErrNotFound, id)
	}
	if err := manager.RemoveSnapshot(ctx, env, id); err != nil {
		return fmt.Errorf("remove snapshot on %s: %w", name, err)
	}
	env.Snapshots = append(env.Snapshots[:snapIdx], env.Snapshots[snapIdx+1:]...)
	envs[idx] = env
	return s.store.Save(envs)
}

// ListApps lists applications a Box's engine can export as a host-visible
// launcher — the Blend Mode base (Architecture → Blend Mode).
func (s *Service) ListApps(ctx context.Context, name string) ([]App, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	exporter, ok := backend.(AppExporter)
	if !ok {
		return nil, Unsupportedf("%s is a Machine: exporting applications is only available for Boxes", name)
	}
	return exporter.ListApps(ctx, env)
}

func (s *Service) ExportApp(ctx context.Context, name, id string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	exporter, ok := backend.(AppExporter)
	if !ok {
		return Unsupportedf("%s is a Machine: exporting applications is only available for Boxes", name)
	}
	if err := exporter.ExportApp(ctx, env, id); err != nil {
		return fmt.Errorf("export app on %s: %w", name, err)
	}
	return nil
}

func (s *Service) UnexportApp(ctx context.Context, name, id string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	exporter, ok := backend.(AppExporter)
	if !ok {
		return Unsupportedf("%s is a Machine: exporting applications is only available for Boxes", name)
	}
	if err := exporter.UnexportApp(ctx, env, id); err != nil {
		return fmt.Errorf("unexport app on %s: %w", name, err)
	}
	return nil
}

// SSH opens a shell in the environment, or runs command there, for
// Backends that implement RemoteShell (Machines).
func (s *Service) SSH(ctx context.Context, name, login string, command []string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	shell, ok := backend.(RemoteShell)
	if !ok {
		return Unsupportedf("%s is a Box: open its shell with omavm open %s", name, name)
	}
	return shell.SSH(ctx, env, login, command)
}

func (s *Service) Exec(ctx context.Context, name string, args []string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if err := backend.Exec(ctx, env, args); err != nil {
		return fmt.Errorf("exec %s: %w", name, err)
	}
	return nil
}

// Remove deletes an environment through its Backend and drops it from
// the store. It only forgets the environment locally if the backend
// confirms removal, so a failed backend removal never leaves state
// pointing at nothing.
func (s *Service) Remove(ctx context.Context, name string) error {
	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return err
	}
	if err := backend.Remove(ctx, env); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	if linker, ok := backend.(HostLinker); ok {
		_ = linker.Unlink(ctx, env)
	}
	// Withdrawn even when disabled: the setting could have been changed
	// by hand, and a leftover entry would open nothing.
	if s.launcher != nil {
		if err := s.launcher.Withdraw(env); err != nil {
			slog.Warn("launcher entry not removed", "environment", env.Name, "error", err)
		}
	}
	envs = append(envs[:idx], envs[idx+1:]...)
	return s.store.Save(envs)
}

func newID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
