package core

import "context"

// State is the observed runtime state of an Environment. Backends report
// their own concrete state through Status.Detail; State stays coarse and
// uniform so the Core and UI don't need per-backend branching.
type State string

const (
	StateRunning State = "running"
	StateStopped State = "stopped"
	StateUnknown State = "unknown"
)

// Status is the observed state of an Environment as reported by its Backend.
type Status struct {
	State  State  `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// Backend executes the lifecycle of environments of one Kind on top of a
// specific engine (Podman/Docker, QEMU/KVM, ...). It is OmaVM's own
// interface, not the underlying tool's API exposed directly: adapters
// translate to and from Podman/Docker/QEMU/KVM without leaking their
// infrastructure details (commands, sockets, qcow2 paths) to the domain
// or above.
//
// Backends must treat Create, Start, Stop and Remove as idempotent where
// the underlying engine allows it, and must never silently degrade an
// operation it cannot support (e.g. Exec on a Machine without a guest
// channel) — return ErrUnsupported instead.
type Backend interface {
	// Name identifies the backend for display and persistence (e.g. "podman", "qemu").
	Name() string

	Create(ctx context.Context, env Environment) error
	Start(ctx context.Context, env Environment) error
	Open(ctx context.Context, env Environment) error
	Stop(ctx context.Context, env Environment) error
	Status(ctx context.Context, env Environment) (Status, error)
	Exec(ctx context.Context, env Environment, args []string) error
	Remove(ctx context.Context, env Environment) error
}
