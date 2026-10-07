package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
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
// Backend turns them into part of a filesystem path. Machines created
// before their directory was named by ID still live under their name
// (internal/backend/machine/qemu's key()), where ".." or "/" would let it escape
// its parent; the launcher and ~/OmaVM links use the name too. Otherwise
// intentionally permissive: spaces, accents and most punctuation are
// fine.
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

// validateCPUs and validateMemory hold a Machine's hardware bounds, the
// same at creation and in Settings.
func validateCPUs(n int) error {
	if n < 1 || n > 64 {
		return Invalidf("cpus must be between 1 and 64")
	}
	return nil
}

func validateMemory(mib int) error {
	if mib < 256 || mib > 262144 {
		return Invalidf("memory-mib must be between 256 and 262144")
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

func indexByID(envs []Environment, id string) int {
	for i, e := range envs {
		if e.ID == id {
			return i
		}
	}
	return -1
}

// lookup finds a registered environment by name without locking anything.
func (s *Service) lookup(name string) (Environment, error) {
	envs, err := s.store.Load()
	if err != nil {
		return Environment{}, err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return Environment{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return env, nil
}

// interrupted reports whether the creation or removal marked on env has
// no process behind it anymore: whoever runs it holds the environment's
// lock until it finishes, and a lock dies with its process.
func (s *Service) interrupted(env Environment) bool {
	unlock, ok, err := s.store.TryLockEnvironment(env.ID)
	if err != nil || !ok {
		return false
	}
	unlock()
	return true
}

// operationError explains why an environment in the middle of its
// creation or removal can't be used.
func operationError(env Environment, interrupted bool) error {
	switch {
	case interrupted && env.Operation == OperationCreating:
		return Invalidf("creating %s was interrupted; delete it and create it again", env.Name)
	case interrupted:
		return Invalidf("deleting %s was interrupted; delete it again", env.Name)
	case env.Operation == OperationCreating:
		return Busyf("%s is still being created", env.Name)
	default:
		return Busyf("%s is being deleted", env.Name)
	}
}

// operationStatus is the status of an environment marked as being
// created or removed, which its Backend can't report yet (or anymore).
func (s *Service) operationStatus(env Environment) Status {
	if s.interrupted(env) {
		return Status{State: StateError, Detail: operationError(env, true).Error()}
	}
	if env.Operation == OperationCreating {
		return Status{State: StateCreating}
	}
	return Status{State: StateRemoving}
}

// acquire takes the named environment's lock for an operation that
// changes it, and returns the environment as registered once the lock is
// held (another process may have changed or removed it meanwhile) with
// its Backend. The registry itself is not held: operations on other
// environments go on.
func (s *Service) acquire(ctx context.Context, name string) (Environment, Backend, func(), error) {
	env, err := s.lookup(name)
	if err != nil {
		return Environment{}, nil, nil, err
	}
	unlock, err := s.store.LockEnvironment(ctx, env.ID)
	if err != nil {
		return Environment{}, nil, nil, err
	}
	envs, err := s.store.Load()
	if err != nil {
		unlock()
		return Environment{}, nil, nil, err
	}
	idx := indexByID(envs, env.ID)
	if idx == -1 {
		unlock()
		return Environment{}, nil, nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	env = envs[idx]
	if env.Operation != "" {
		// Its lock is ours, so nobody is running that operation.
		unlock()
		return Environment{}, nil, nil, operationError(env, true)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		unlock()
		return Environment{}, nil, nil, err
	}
	return env, backend, unlock, nil
}

// update changes one environment under the registry lock, on a freshly
// loaded registry, so a concurrent change to another field (a Configure
// while a snapshot is taken) is never overwritten by a stale copy.
func (s *Service) update(ctx context.Context, id string, change func(*Environment)) (Environment, error) {
	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return Environment{}, err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return Environment{}, err
	}
	idx := indexByID(envs, id)
	if idx == -1 {
		return Environment{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	change(&envs[idx])
	if err := s.store.Save(envs); err != nil {
		return Environment{}, err
	}
	return envs[idx], nil
}

// drop removes one environment from the registry.
func (s *Service) drop(ctx context.Context, id string) error {
	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return err
	}
	idx := indexByID(envs, id)
	if idx == -1 {
		return nil
	}
	return s.store.Save(append(envs[:idx], envs[idx+1:]...))
}

// List returns every known environment, regardless of Kind or Backend.
func (s *Service) List(ctx context.Context) ([]Environment, error) {
	return s.store.Load()
}

// EnvironmentState is an Environment together with what its Backend
// reports about it right now.
type EnvironmentState struct {
	Environment
	Status Status `json:"status"`
	// Integration is only reported for environments whose Backend checks
	// guest tools (Machines).
	Integration *IntegrationReport `json:"integration,omitempty"`
}

// ListWithStatus is List plus every environment's current status, in one
// call: Backends that implement StatusLister answer for all their
// environments at once, the others are asked concurrently. A failure to
// get one environment's status is reported as its status, never as a
// failure of the whole list.
func (s *Service) ListWithStatus(ctx context.Context) ([]EnvironmentState, error) {
	envs, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	states := make([]EnvironmentState, len(envs))
	byKind := map[EnvironmentKind][]int{}
	for i, env := range envs {
		states[i].Environment = env
		if env.Operation != "" {
			states[i].Status = s.operationStatus(env)
			continue
		}
		byKind[env.Kind] = append(byKind[env.Kind], i)
	}

	var wg sync.WaitGroup
	for kind, indexes := range byKind {
		backend, err := s.backendFor(kind)
		if err != nil {
			for _, i := range indexes {
				states[i].Status = Status{State: StateUnknown, Detail: err.Error()}
			}
			continue
		}
		if lister, ok := backend.(StatusLister); ok {
			group := make([]Environment, len(indexes))
			for j, i := range indexes {
				group[j] = envs[i]
			}
			statuses, err := lister.Statuses(ctx, group)
			for _, i := range indexes {
				status, found := statuses[envs[i].ID]
				switch {
				case err != nil:
					status = Status{State: StateUnknown, Detail: err.Error()}
				case !found:
					status = Status{State: StateUnknown}
				}
				states[i].Status = status
			}
		}
		reporter, _ := backend.(IntegrationReporter)
		for _, i := range indexes {
			wg.Add(1)
			go func(i int, backend Backend) {
				defer wg.Done()
				state := &states[i]
				if _, batched := backend.(StatusLister); !batched {
					status, err := backend.Status(ctx, state.Environment)
					if err != nil {
						status = Status{State: StateUnknown, Detail: err.Error()}
					}
					state.Status = status
				}
				if reporter != nil {
					if report, err := reporter.Integration(ctx, state.Environment); err == nil {
						state.Integration = &report
					}
				}
			}(i, backend)
		}
	}
	wg.Wait()
	return states, nil
}

// Create registers a new environment and delegates its creation to the
// Backend responsible for its Kind.
func (s *Service) Create(ctx context.Context, env Environment) (Environment, error) {
	env.Name = strings.TrimSpace(env.Name)
	if err := validateEnvironmentName(env.Name); err != nil {
		return Environment{}, err
	}
	// Checked here rather than left to the (slower) backend.Create: a
	// Machine starts with an empty disk, so without installation media it
	// has nothing to boot, and nothing can attach an ISO later. Without an
	// image, `distrobox create --image ""` never returns.
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
	if env.Settings.CPUs != 0 || env.Settings.MemoryMiB != 0 {
		if env.Kind != Machine {
			return Environment{}, Unsupportedf("CPU and memory settings only apply to Machines")
		}
		// 0 means the default.
		if env.Settings.CPUs != 0 {
			if err := validateCPUs(env.Settings.CPUs); err != nil {
				return Environment{}, err
			}
		}
		if env.Settings.MemoryMiB != 0 {
			if err := validateMemory(env.Settings.MemoryMiB); err != nil {
				return Environment{}, err
			}
		}
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
	env.Backend = backend.Name()

	// The name is reserved in the registry for as long as the backend
	// takes (pulling a Box's image can take minutes), with only this
	// environment's lock held meanwhile, not the registry's: other
	// environments stay usable. The lock is taken before the reservation
	// appears, so nobody can mistake it for an interrupted creation.
	unlockEnv, err := s.store.LockEnvironment(ctx, id)
	if err != nil {
		return Environment{}, err
	}
	defer unlockEnv()
	if err := s.reserve(ctx, env); err != nil {
		return Environment{}, err
	}

	if err := backend.Create(ctx, env); err != nil {
		if dropErr := s.drop(ctx, id); dropErr != nil {
			return Environment{}, fmt.Errorf("create %s: %w (and it could not be taken out of the registry: %v)", env.Name, err, dropErr)
		}
		return Environment{}, fmt.Errorf("create %s: %w", env.Name, err)
	}
	created, err := s.update(ctx, id, func(e *Environment) { e.Operation = "" })
	if err != nil {
		// The backend created a real resource (a container, a VM disk)
		// but the registry can't say so: undo it rather than leave it
		// orphaned and invisible to the CLI/GUI.
		removeErr := backend.Remove(ctx, env)
		_ = s.drop(ctx, id)
		if removeErr != nil {
			return Environment{}, fmt.Errorf("save registry after creating %s: %w (cleanup also failed, resource may be orphaned: %v)", env.Name, err, removeErr)
		}
		return Environment{}, fmt.Errorf("save registry after creating %s: %w", env.Name, err)
	}
	s.syncLauncher(created)
	return created, nil
}

// Update updates the software installed in a Box with its package
// manager. The Box's lock is held throughout, like Start.
func (s *Service) Update(ctx context.Context, name string) error {
	env, backend, unlock, err := s.acquire(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	upgrader, ok := backend.(Upgrader)
	if !ok {
		return Unsupportedf("%s is a Desktop: update it from inside its own system", name)
	}
	if err := upgrader.Upgrade(ctx, env); err != nil {
		return fmt.Errorf("update %s: %w", name, err)
	}
	return nil
}

// Clone makes newName a full copy of name, as it is now: same kind,
// image and settings, and for a Machine its disk with every snapshot in
// it. The source stays locked while it is copied, so nothing starts it
// halfway through; the copy is registered like a creation (reserved
// first, dropped again if the backend fails).
func (s *Service) Clone(ctx context.Context, name, newName string) (Environment, error) {
	newName = strings.TrimSpace(newName)
	if err := validateEnvironmentName(newName); err != nil {
		return Environment{}, err
	}
	source, backend, unlockSource, err := s.acquire(ctx, name)
	if err != nil {
		return Environment{}, err
	}
	defer unlockSource()
	cloner, ok := backend.(Cloner)
	if !ok {
		return Environment{}, Unsupportedf("%s can't be cloned", name)
	}
	id, err := newID()
	if err != nil {
		return Environment{}, err
	}
	clone := Environment{
		ID:        id,
		Name:      newName,
		Image:     source.Image,
		Backend:   source.Backend,
		Kind:      source.Kind,
		Settings:  source.Settings,
		Snapshots: append([]Snapshot(nil), source.Snapshots...),
	}
	unlockClone, err := s.store.LockEnvironment(ctx, id)
	if err != nil {
		return Environment{}, err
	}
	defer unlockClone()
	if err := s.reserve(ctx, clone); err != nil {
		return Environment{}, err
	}
	if err := cloner.Clone(ctx, source, clone); err != nil {
		if dropErr := s.drop(ctx, id); dropErr != nil {
			return Environment{}, fmt.Errorf("clone %s: %w (and %s could not be taken out of the registry: %v)", name, err, newName, dropErr)
		}
		return Environment{}, fmt.Errorf("clone %s: %w", name, err)
	}
	created, err := s.update(ctx, id, func(e *Environment) { e.Operation = "" })
	if err != nil {
		removeErr := backend.Remove(ctx, clone)
		_ = s.drop(ctx, id)
		if removeErr != nil {
			return Environment{}, fmt.Errorf("save registry after cloning %s: %w (cleanup also failed, resource may be orphaned: %v)", name, err, removeErr)
		}
		return Environment{}, fmt.Errorf("save registry after cloning %s: %w", name, err)
	}
	s.syncLauncher(created)
	return created, nil
}

// reserve adds env to the registry marked as being created, if its name
// is free.
func (s *Service) reserve(ctx context.Context, env Environment) error {
	unlock, err := s.store.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	envs, err := s.store.Load()
	if err != nil {
		return err
	}
	if _, idx := findEnvironment(envs, env.Name); idx != -1 {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, env.Name)
	}
	env.Operation = OperationCreating
	return s.store.Save(append(envs, env))
}

// resolve finds an environment for an operation that doesn't change it
// (status, open, exec, ...), and so takes no lock.
func (s *Service) resolve(ctx context.Context, name string) (Environment, Backend, error) {
	env, err := s.lookup(name)
	if err != nil {
		return Environment{}, nil, err
	}
	if env.Operation != "" {
		return Environment{}, nil, operationError(env, s.interrupted(env))
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return Environment{}, nil, err
	}
	return env, backend, nil
}

func (s *Service) Start(ctx context.Context, name string) error {
	env, backend, unlock, err := s.acquire(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	if err := backend.Start(ctx, env); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	// Also publishes environments created before launcher entries
	// existed, the first time they are used.
	s.syncLauncher(env)
	return nil
}

// StartEphemeral starts a session whose changes are discarded at shutdown.
// The launcher entry isn't touched: it opens the environment normally.
func (s *Service) StartEphemeral(ctx context.Context, name string) error {
	env, backend, unlock, err := s.acquire(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	starter, ok := backend.(EphemeralStarter)
	if !ok {
		return Unsupportedf("%s can't start without keeping changes: only Desktops (Machines) can", name)
	}
	if err := starter.StartEphemeral(ctx, env); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	return nil
}

// Open takes no lock: a Box's Open is its interactive shell and lasts as
// long as the user keeps it.
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
	env, backend, unlock, err := s.acquire(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	if err := backend.Stop(ctx, env); err != nil {
		return fmt.Errorf("stop %s: %w", name, err)
	}
	return nil
}

// advancedLifecycle is acquire for the Machine-only controls; the caller
// releases the lock.
func (s *Service) advancedLifecycle(ctx context.Context, name string) (Environment, AdvancedLifecycle, func(), error) {
	env, backend, unlock, err := s.acquire(ctx, name)
	if err != nil {
		return Environment{}, nil, nil, err
	}
	lifecycle, ok := backend.(AdvancedLifecycle)
	if !ok {
		unlock()
		return Environment{}, nil, nil, Unsupportedf("%s is a Box: restart, pause, resume and force stop are only available for Machines", name)
	}
	return env, lifecycle, unlock, nil
}

func (s *Service) Restart(ctx context.Context, name string) error {
	env, lifecycle, unlock, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	return lifecycle.Restart(ctx, env)
}

func (s *Service) Pause(ctx context.Context, name string) error {
	env, lifecycle, unlock, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	return lifecycle.Pause(ctx, env)
}

func (s *Service) Resume(ctx context.Context, name string) error {
	env, lifecycle, unlock, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	return lifecycle.Resume(ctx, env)
}

func (s *Service) ForceStop(ctx context.Context, name string) error {
	env, lifecycle, unlock, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	return lifecycle.ForceStop(ctx, env)
}

func (s *Service) Status(ctx context.Context, name string) (Status, error) {
	env, err := s.lookup(name)
	if err != nil {
		return Status{}, err
	}
	if env.Operation != "" {
		return s.operationStatus(env), nil
	}
	backend, err := s.backendFor(env.Kind)
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

// PrepareGuest sets up the guest side of a Machine's integrations. Like
// Exec, it takes no lock: it acts inside a running guest and can take
// minutes (installing packages), and holding the environment's lock that
// long would block a Stop.
func (s *Service) PrepareGuest(ctx context.Context, name string) ([]PreparationStep, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	preparer, ok := backend.(GuestPreparer)
	if !ok {
		return nil, Unsupportedf("%s is a Box: it already shares your home folder, clipboard and display with this computer", name)
	}
	return preparer.PrepareGuest(ctx, env)
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
	if env.Operation != "" {
		// TryLock never waits, so trying the environment's lock while
		// holding the registry's can't deadlock.
		return EnvironmentSettings{}, operationError(env, s.interrupted(env))
	}
	// Start from the raw, persisted settings — not EffectiveSettings().
	// EffectiveSettings() substitutes defaults for CPUs/MemoryMiB/
	// SnapshotLimit (0 -> 2/2048/10) purely for display; starting the
	// patch from that resolved copy would silently re-persist those
	// defaults as if explicitly pinned on every single Configure call,
	// even ones that only touch an unrelated field like Description.
	// That's a real bug this project hit: it permanently disables Travel
	// Mode's automatic CPU reduction (internal/backend/machine/qemu/qemu.go's
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
		// Characters, not bytes: the Settings dialog counts characters,
		// and an accented letter takes two bytes.
		if utf8.RuneCountInString(settings.Description) > 500 {
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
		if err := validateCPUs(*patch.CPUs); err != nil {
			return EnvironmentSettings{}, err
		}
		settings.CPUs = *patch.CPUs
	}
	if patch.MemoryMiB != nil {
		if err := validateMemory(*patch.MemoryMiB); err != nil {
			return EnvironmentSettings{}, err
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
	if patch.ClipboardDirection != nil {
		switch direction := strings.TrimSpace(*patch.ClipboardDirection); direction {
		case "", ClipboardBoth:
			settings.ClipboardDirection = ""
		case ClipboardToHost:
			settings.ClipboardDirection = direction
		case ClipboardToGuest:
			if env.Kind != Machine {
				return EnvironmentSettings{}, Unsupportedf("a Box's clipboard only goes to this computer (programs copying from its terminal); turn it off instead")
			}
			settings.ClipboardDirection = direction
		default:
			return EnvironmentSettings{}, Invalidf("clipboard direction must be one of: %s, %s, %s", ClipboardBoth, ClipboardToHost, ClipboardToGuest)
		}
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
	if patch.Fullscreen != nil {
		if env.Kind != Machine {
			return EnvironmentSettings{}, Unsupportedf("fullscreen only applies to a Machine's display; a Box opens as a terminal")
		}
		settings.FullscreenDisabled = !*patch.Fullscreen
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
				if _, err := linker.Link(ctx, env, settings.Color); err != nil {
					// ~/OmaVM/<name> taken by a file of the user's own,
					// for example: the color still holds inside OmaVM.
					slog.Warn("host link not updated", "environment", env.Name, "error", err)
				}
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
	env, manager, unlock, err := s.snapshotManager(ctx, name)
	if err != nil {
		return Snapshot{}, err
	}
	defer unlock()

	tag, err := snapshotTag(label)
	if err != nil {
		return Snapshot{}, err
	}
	snap, err := manager.CreateSnapshot(ctx, env, tag)
	if err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot %s: %w", name, err)
	}
	snap.ID, snap.Label, snap.CreatedAt = tag, label, time.Now().UTC()

	// Recorded right after the backend confirms it, before retention: if
	// a discard below fails, the registry must still know this snapshot,
	// or it would exist in the disk with no way to reach it.
	env, err = s.update(ctx, env.ID, func(e *Environment) { e.Snapshots = append(e.Snapshots, snap) })
	if err != nil {
		return Snapshot{}, err
	}

	limit := env.EffectiveSettings().SnapshotLimit
	for limit > 0 && len(env.Snapshots) > limit {
		oldest := env.Snapshots[0]
		if err := manager.RemoveSnapshot(ctx, env, oldest.ID); err != nil && !errors.Is(err, ErrSnapshotGone) {
			// snap itself is saved; only the retention discard failed.
			return snap, fmt.Errorf("created %s, but discarding the oldest snapshot for %s failed: %w", label, name, err)
		}
		// Each discard is recorded at once: the backend has already
		// deleted it, so a later failure must not leave it listed.
		env, err = s.update(ctx, env.ID, func(e *Environment) { e.Snapshots = withoutSnapshot(e.Snapshots, oldest.ID) })
		if err != nil {
			return snap, err
		}
	}
	return snap, nil
}

// snapshotManager is acquire for snapshot operations; the caller
// releases the lock. Holding the environment's lock across the backend
// call serializes snapshot operations on one Machine (reproduced
// 2026-09-27: a lock-free go-to racing a create hit qemu-img's own image
// lock with a raw "Failed to get \"write\" lock").
func (s *Service) snapshotManager(ctx context.Context, name string) (Environment, SnapshotManager, func(), error) {
	env, backend, unlock, err := s.acquire(ctx, name)
	if err != nil {
		return Environment{}, nil, nil, err
	}
	manager, ok := backend.(SnapshotManager)
	if !ok {
		unlock()
		return Environment{}, nil, nil, Unsupportedf("%s is a Box: snapshots are only available for Machines for now", name)
	}
	return env, manager, unlock, nil
}

func withoutSnapshot(snapshots []Snapshot, id string) []Snapshot {
	out := make([]Snapshot, 0, len(snapshots))
	for _, snap := range snapshots {
		if snap.ID != id {
			out = append(out, snap)
		}
	}
	return out
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
func (s *Service) GoToSnapshot(ctx context.Context, name, id string) error {
	env, manager, unlock, err := s.snapshotManager(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	if findSnapshot(env.Snapshots, id) == -1 {
		return fmt.Errorf("%w: snapshot %s", ErrNotFound, id)
	}
	if err := manager.GoToSnapshot(ctx, env, id); err != nil {
		return fmt.Errorf("go to snapshot on %s: %w", name, err)
	}
	return nil
}

func (s *Service) RemoveSnapshot(ctx context.Context, name, id string) error {
	env, manager, unlock, err := s.snapshotManager(ctx, name)
	if err != nil {
		return err
	}
	defer unlock()
	if findSnapshot(env.Snapshots, id) == -1 {
		return fmt.Errorf("%w: snapshot %s", ErrNotFound, id)
	}
	if err := manager.RemoveSnapshot(ctx, env, id); err != nil && !errors.Is(err, ErrSnapshotGone) {
		return fmt.Errorf("remove snapshot on %s: %w", name, err)
	} else if err != nil {
		// Already gone from the disk: only the registry still lists it.
		slog.Warn("snapshot was no longer in the disk; removed from the list", "environment", name, "snapshot", id, "error", err)
	}
	_, err = s.update(ctx, env.ID, func(e *Environment) { e.Snapshots = withoutSnapshot(e.Snapshots, id) })
	return err
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
// the registry. The environment is marked as being removed for the whole
// backend call, and only forgotten once the backend confirms, so a failed
// removal never leaves the registry pointing at nothing. An environment
// whose creation or removal was interrupted can always be removed.
func (s *Service) Remove(ctx context.Context, name string) error {
	env, err := s.lookup(name)
	if err != nil {
		return err
	}
	unlockEnv, err := s.store.LockEnvironment(ctx, env.ID)
	if err != nil {
		return err
	}
	defer unlockEnv()
	var previous string
	env, err = s.update(ctx, env.ID, func(e *Environment) {
		previous = e.Operation
		e.Operation = OperationRemoving
	})
	if err != nil {
		return err
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		_, _ = s.update(ctx, env.ID, func(e *Environment) { e.Operation = previous })
		return err
	}
	if err := backend.Remove(ctx, env); err != nil {
		if previous != OperationCreating {
			_, _ = s.update(ctx, env.ID, func(e *Environment) { e.Operation = previous })
			return fmt.Errorf("remove %s: %w", name, err)
		}
		// An interrupted creation may never have got as far as the
		// resource the backend fails to find now.
		slog.Warn("removing an interrupted creation", "environment", name, "error", err)
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
	return s.drop(ctx, env.ID)
}

func newID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
