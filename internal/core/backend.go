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
)

// Status is the observed state of an Environment as reported by its Backend.
type Status struct {
	State  State  `json:"state"`
	Detail string `json:"detail,omitempty"`
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
}

// SnapshotManager is an optional Backend capability for environments whose
// engine supports point-in-time state capture natively (QEMU/qcow2 internal
// snapshots for Machines). The Core owns the meaningful Label and the
// history (Environment.Snapshots); the Backend only executes against the
// technical tag the Core generated. A Backend without this capability
// (e.g. Box today) makes Service.CreateSnapshot/GoToSnapshot/RemoveSnapshot
// fail with ErrUnsupported rather than pretending to support it.
type SnapshotManager interface {
	CreateSnapshot(ctx context.Context, env Environment, tag string) error
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
