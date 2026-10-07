package core

import "context"

// State is the observed runtime state of an Environment. Backends report
// their own concrete state through Status.Detail; State stays coarse and
// uniform so the Core and UI don't need per-backend branching.
type State string

const (
	StateRunning  State = "running"
	StatePaused   State = "paused"
	StateStarting State = "starting"
	StateStopping State = "stopping"
	StateStopped  State = "stopped"
	StateError    State = "error"
	StateUnknown  State = "unknown"
	// StateCreating and StateRemoving are the Core's own: the environment
	// is in the registry while its Backend creates or removes it.
	StateCreating State = "creating"
	StateRemoving State = "removing"
)

// Status is the observed state of an Environment as reported by its Backend.
type Status struct {
	State  State  `json:"state"`
	Detail string `json:"detail,omitempty"`
	// Ephemeral means this session's changes are thrown away when the
	// environment shuts down (see EphemeralStarter).
	Ephemeral bool `json:"ephemeral,omitempty"`
	// Warning is something about to go wrong that the state doesn't show,
	// such as the host's disk running out under a Machine.
	Warning string `json:"warning,omitempty"`
	// RestartNeeded means saved settings differ from what the running
	// session got; they apply the next time it starts.
	RestartNeeded bool `json:"restart_needed,omitempty"`
	// TravelMode means the session runs with fewer CPUs than its setting
	// because the host was on battery when it started.
	TravelMode bool `json:"travel_mode,omitempty"`
}

// Backend executes the lifecycle of environments of one Kind on top of a
// specific integration (Distrobox, QEMU/KVM, ...). It is OmaVM's own
// interface, not the underlying tool's API exposed directly: adapters
// translate to and from Distrobox/Podman/Docker/QEMU/KVM without leaking their
// infrastructure details (commands, sockets, qcow2 paths) to the domain
// or above.
//
// Backends must treat Create, Start, Stop and Remove as idempotent where
// the underlying engine allows it, and must never silently degrade an
// operation it cannot support (e.g. Exec on a Machine without a guest
// channel) — return ErrUnsupported instead.
type Backend interface {
	// Name identifies the backend for display and persistence (e.g. "distrobox", "qemu").
	Name() string

	Create(ctx context.Context, env Environment) error
	Start(ctx context.Context, env Environment) error
	Open(ctx context.Context, env Environment) error
	Stop(ctx context.Context, env Environment) error
	Status(ctx context.Context, env Environment) (Status, error)
	Exec(ctx context.Context, env Environment, args []string) error
	Remove(ctx context.Context, env Environment) error
}

// StatusLister is an optional Backend capability: the status of many
// environments from one query to the engine, instead of one per
// environment. The GUI asks for every status every few seconds, and a
// container engine answers for all its containers as cheaply as for one.
// The result is keyed by Environment.ID; an environment missing from it is
// reported as unknown.
type StatusLister interface {
	Statuses(ctx context.Context, envs []Environment) (map[string]Status, error)
}

// Previewer is an optional Backend capability: a screenshot of the
// environment's current graphical state. Only Machines can implement
// it — a Box has no display to preview, which is a real, honest
// difference (Security Model), not a gap to paper over with a fake
// placeholder pretending equivalence. Service.Preview returns
// ErrUnsupported for a Backend that doesn't implement this.
type Previewer interface {
	// Preview returns a filesystem path to an image (format left to the
	// Backend; the qemu adapter writes PPM, which gdk-pixbuf loads
	// natively) representing the environment's current display.
	Preview(ctx context.Context, env Environment) (imagePath string, err error)
}

// AdvancedLifecycle is an optional Backend capability for environments that
// can be controlled beyond the common start/open/stop lifecycle.
type AdvancedLifecycle interface {
	Restart(ctx context.Context, env Environment) error
	Pause(ctx context.Context, env Environment) error
	Resume(ctx context.Context, env Environment) error
	ForceStop(ctx context.Context, env Environment) error
}

// IntegrationReporter is an optional Backend capability that reports whether
// host/guest integration components are actually reachable.
type IntegrationReporter interface {
	Integration(ctx context.Context, env Environment) (IntegrationReport, error)
}

type IntegrationReport struct {
	GuestAgent string `json:"guest_agent"`
	Hint       string `json:"hint,omitempty"`
	// Capabilities says, per host↔guest integration, whether it works,
	// as far as OmaVM can check, and the next step when it doesn't.
	Capabilities []GuestCapability `json:"capabilities,omitempty"`
}

// GuestCapability is one integration with the guest (clipboard, shared
// folder). State is ready only after a real check of the guest side; a
// setting being on is not proof that it works.
type GuestCapability struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	State string `json:"state"`
	// Hint is the concrete next step, or what "ready" means in use.
	Hint string `json:"hint,omitempty"`
}

// GuestCapability states.
const (
	GuestReady          = "ready"                 // checked on the guest side
	GuestNeedsComponent = "needs_guest_component" // something to install or do in the guest
	GuestNeedsRestart   = "needs_restart"         // saved, applies on the next start
	GuestNotVerified    = "not_verified"          // can't be checked right now
	GuestOff            = "off"                   // turned off, or not set up
)

// GuestPreparer is an optional Backend capability that sets up the guest
// side of the integrations (the clipboard agent, the shared folder)
// through an agent already running in the guest, and checks what it can't
// set up. It changes the guest system, so it runs only when asked, never
// on its own.
type GuestPreparer interface {
	PrepareGuest(ctx context.Context, env Environment) ([]PreparationStep, error)
}

// PreparationStep is what preparing the guest did for one integration.
type PreparationStep struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Result string `json:"result"`
	// Detail says what was done, what is still needed, or why it failed.
	Detail string `json:"detail,omitempty"`
}

// PreparationStep results.
const (
	StepDone    = "done"    // set up now
	StepReady   = "ready"   // already worked, nothing to do
	StepManual  = "manual"  // needs something only the person can do
	StepSkipped = "skipped" // turned off, or not set up in Settings
	StepFailed  = "failed"
)

// SnapshotManager is an optional Backend capability for environments whose
// engine supports point-in-time state capture natively (QEMU/qcow2 internal
// snapshots for Machines). The Core owns the meaningful Label and the
// history (Environment.Snapshots); the Backend only executes against the
// technical tag the Core generated. A Backend without this capability
// (e.g. Box today) makes Service.CreateSnapshot/GoToSnapshot/RemoveSnapshot
// fail with ErrUnsupported rather than pretending to support it.
type SnapshotManager interface {
	// CreateSnapshot reports what the snapshot does and doesn't hold
	// (CrashConsistent, WithoutFirmwareState); the Core fills in its ID,
	// Label and CreatedAt.
	CreateSnapshot(ctx context.Context, env Environment, tag string) (Snapshot, error)
	GoToSnapshot(ctx context.Context, env Environment, tag string) error
	RemoveSnapshot(ctx context.Context, env Environment, tag string) error
}

// App is an application discovered inside a Box that can be exported as a
// normal-looking launcher on the host — the first step toward Blend Mode
// (Architecture → Blend Mode). ID is the Backend's own handle for it
// (for Distrobox, the absolute path to the .desktop file inside the Box:
// distrobox-export accepts that directly and it sidesteps the ambiguity
// of matching by app name); Name is what CLI/GUI show a human.
type App struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Exported bool   `json:"exported"`
}

// AppExporter is an optional Backend capability for environments whose
// engine can natively export an installed application as a host-visible
// launcher (distrobox-export for Boxes). This is the Blend Mode base:
// reusing a mature Distrobox feature (Backend Rules), not building a
// window-integration mechanism of OmaVM's own. Machines don't implement
// this — seamless guest window integration is the gated "complete" Blend
// Mode, not the base.
type AppExporter interface {
	ListApps(ctx context.Context, env Environment) ([]App, error)
	ExportApp(ctx context.Context, env Environment, id string) error
	UnexportApp(ctx context.Context, env Environment, id string) error
}

// HostLinker is an optional Backend capability that gives an Environment a
// visible presence on the host filesystem (e.g. a Machine's disk image
// exposed under ~/OmaVM/<name>), used to carry a Color tag into the host
// file manager on a best-effort basis. Not implementing it is not an
// error: Service.Configure only calls it when present.
type HostLinker interface {
	// Link ensures the host-visible path exists (creating/updating it as
	// needed) and applies the given color tag to it best-effort. Returns
	// the path for display/automation.
	Link(ctx context.Context, env Environment, color string) (path string, err error)
	Unlink(ctx context.Context, env Environment) error
}

// RemoteShell is an optional Backend capability: a shell or command in the
// environment over SSH (Machines, through a host↔guest socket that needs
// no guest network). login "" means the host user's name.
type RemoteShell interface {
	SSH(ctx context.Context, env Environment, login string, command []string) error
}

// Upgrader is an optional Backend capability: update the software installed
// in the environment with its own package manager (distrobox upgrade for
// Boxes). A Machine updates from inside its own system instead.
type Upgrader interface {
	Upgrade(ctx context.Context, env Environment) error
}

// Cloner is an optional Backend capability: make clone a full, independent
// copy of source (its disk or container), as source is now. Both stay
// usable afterwards; nothing is shared between them. A backend refuses a
// source that is running, whose state would be copied half-written.
type Cloner interface {
	Clone(ctx context.Context, source, clone Environment) error
}

// EphemeralStarter is an optional Backend capability: start a session
// whose changes are discarded when it shuts down, leaving the environment
// exactly as it was. Nothing is persisted in the domain: it is a property
// of one session, reported back through Status.Ephemeral. Starting an
// environment already running a normal session is ErrInvalidInput, so a
// caller never believes changes will vanish when they won't.
type EphemeralStarter interface {
	StartEphemeral(ctx context.Context, env Environment) error
}

// Launcher publishes environments in the host's application launcher, so
// one opens like any installed app without going through the Experience
// Center first. It is host desktop integration rather than a Backend
// capability: the same entry works for both Kinds. On by default,
// withdrawn per environment with EnvironmentSettings.LauncherDisabled.
// Failures never fail the operation that triggered them.
type Launcher interface {
	// Publish creates or updates the environment's entry; it must be
	// idempotent and cheap, since Start and Open call it every time.
	Publish(env Environment) error
	Withdraw(env Environment) error
}

// HostInspector is an optional Backend capability reporting what the host
// can offer that Backend's Environments (hardware virtualization, graphics
// acceleration, ...). It only reads the host and never changes it.
type HostInspector interface {
	InspectHost(ctx context.Context) ([]HostCapability, error)
}

// HostCapability is one host feature, described by what it gives the user
// rather than by the mechanism behind it.
type HostCapability struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
	Hint      string `json:"hint,omitempty"`
}
