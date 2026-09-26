// Package core is the OmaVM Core: it models environments and delegates
// their lifecycle to a Backend, without depending on any specific
// virtualization or container tool.
package core

import (
	"encoding/json"
	"fmt"
)

// EnvironmentKind distinguishes a userspace-only Box from a Machine with
// an independent kernel. It is a semantic distinction, not a distro
// distinction: Linux can be a Machine when it needs its own kernel.
type EnvironmentKind int

const (
	Box EnvironmentKind = iota
	Machine
)

func (k EnvironmentKind) String() string {
	switch k {
	case Box:
		return "box"
	case Machine:
		return "machine"
	default:
		return "unknown"
	}
}

// ParseEnvironmentKind parses the CLI/GUI-facing spelling of a kind.
func ParseEnvironmentKind(s string) (EnvironmentKind, error) {
	switch s {
	case "box":
		return Box, nil
	case "machine":
		return Machine, nil
	default:
		return 0, &InvalidKindError{Value: s}
	}
}

// MarshalJSON renders as "box"/"machine" rather than a raw int, so both
// the on-disk state file and CLI --json output stay human-readable and
// stable across a future reordering of the const block.
func (k EnvironmentKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

func (k *EnvironmentKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		parsed, err := ParseEnvironmentKind(s)
		if err != nil {
			return err
		}
		*k = parsed
		return nil
	}

	// Accept the pre-MarshalJSON raw-int encoding too, so an
	// environments.json written before this format existed still loads.
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("unmarshal environment kind: %w", err)
	}
	if n != int(Box) && n != int(Machine) {
		return fmt.Errorf("unmarshal environment kind: out of range: %d", n)
	}
	*k = EnvironmentKind(n)
	return nil
}

// Environment is a computational environment the user creates, runs and
// manages. Backend is the name of the Backend that owns it (e.g.
// "podman", "qemu"); it is set by the Service on Create and is an
// implementation detail that higher layers may display but never branch
// product behavior on beyond what Kind already implies.
type Environment struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Image   string          `json:"image"`
	Backend string          `json:"backend"`
	Kind    EnvironmentKind `json:"kind"`
}
