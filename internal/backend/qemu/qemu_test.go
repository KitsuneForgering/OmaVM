package qemu

import (
	"testing"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

func TestVNCPathIsUnderMachineStateDir(t *testing.T) {
	b := &Backend{stateDir: t.TempDir()}
	name := "fedora"

	got := b.vncPath(name)
	want := b.dir(name) + "/vnc.sock"
	if got != want {
		t.Fatalf("vncPath(%q) = %q, want %q", name, got, want)
	}
}

func TestStatusFromQMP(t *testing.T) {
	tests := map[string]core.State{
		"running":        core.StateRunning,
		"paused":         core.StatePaused,
		"suspended":      core.StatePaused,
		"inmigrate":      core.StateStarting,
		"shutdown":       core.StateStopping,
		"guest-panicked": core.StateError,
		"colo":           core.StateUnknown,
	}
	for input, want := range tests {
		if got := statusFromQMP(input).State; got != want {
			t.Errorf("statusFromQMP(%q) = %q, want %q", input, got, want)
		}
	}
}
