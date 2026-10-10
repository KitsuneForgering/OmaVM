package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

func TestErrorClassesHaveStableExitCodes(t *testing.T) {
	tests := []struct {
		err      error
		wantExit int
		wantCode string
	}{
		{errors.New("qemu crashed"), 1, "failed"},
		{usagef("status: expected exactly one environment name"), 2, "invalid_input"},
		{core.Invalidf("name must not contain /"), 2, "invalid_input"},
		{&core.InvalidKindError{Value: "vm"}, 2, "invalid_input"},
		{fmt.Errorf("start x: %w", core.ErrNotFound), 3, "not_found"},
		{core.ErrAlreadyExists, 4, "already_exists"},
		{core.Unsupportedf("x is a Box"), 5, "unsupported"},
		{core.Busyf("x is being created"), 6, "busy"},
	}
	for _, tt := range tests {
		exit, code := errorClass(tt.err)
		if exit != tt.wantExit || code != tt.wantCode {
			t.Errorf("errorClass(%v) = %d, %q; want %d, %q", tt.err, exit, code, tt.wantExit, tt.wantCode)
		}
	}
}

func TestUsageErrorsFromRunAreInvalidInput(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, args := range [][]string{
		{"frobnicate"},
		{"status"},
		{"create", "--kind", "box"},
		{"create", "--name", "x", "--bogus"},
		{"snapshot", "create", "vm", "--label"},
	} {
		if exit, _ := errorClass(run(args)); exit != exitUsage {
			t.Errorf("run(%q) exit = %d, want %d", args, exit, exitUsage)
		}
	}
	if exit, _ := errorClass(run([]string{"status", "nothing-here"})); exit != exitNotFound {
		t.Errorf("status of a missing environment exit = %d, want %d", exit, exitNotFound)
	}
}

func TestExecPassesTheCommandExitCodeThrough(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 7").Run()
	wrapped := fmt.Errorf("exec box: %w", err)
	if got := exitCodeFor("exec", wrapped); got != 7 {
		t.Fatalf("exec exit = %d, want 7", got)
	}
	// Elsewhere a failing helper is just a failure of omavm.
	if got := exitCodeFor("start", wrapped); got != exitFailure {
		t.Fatalf("start exit = %d, want %d", got, exitFailure)
	}
}

func TestJSONErrorShape(t *testing.T) {
	var buf bytes.Buffer
	writeJSONError(&buf, fmt.Errorf("status x: %w", core.ErrNotFound))
	var got map[string]map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"]["code"] != "not_found" || got["error"]["message"] != "status x: environment not found" {
		t.Fatalf("unexpected JSON error: %s", buf.String())
	}
}

func TestWantsJSONIgnoresTheExecutedCommand(t *testing.T) {
	if !wantsJSON([]string{"status", "x", "--json"}) {
		t.Error("--json after the name not seen")
	}
	if wantsJSON([]string{"exec", "x", "--", "tool", "--json"}) {
		t.Error("--json of the command run by exec taken as omavm's")
	}
}

// The command exec ran failing is reported by its own code and its own
// words only: omavm used to add "omavm: exec box: distrobox enter
// --no-tty --name omavm-box-… -- false: exit status 1", naming the
// backend's internals.
func TestCommandExitIsTheCommandsNotOmavms(t *testing.T) {
	failed := fmt.Errorf("exec box: %w", exec.Command("false").Run())
	if code, ok := commandExit("exec", failed); !ok || code != 1 {
		t.Fatalf("exec of a failing command: %d, %v", code, ok)
	}
	if _, ok := commandExit("ssh", failed); !ok {
		t.Fatal("ssh passes the remote code through too")
	}
	if _, ok := commandExit("start", failed); ok {
		t.Fatal("a helper failing under start is omavm failing")
	}
	if _, ok := commandExit("exec", errors.New("box not found")); ok {
		t.Fatal("exec failing before the command ran is omavm failing")
	}
}

// omavm open passes a Box shell's own exit status on silently; anything
// else failing under open (QEMU not starting for a Machine) is omavm's
// failure and keeps its message.
func TestOpenPassesTheSessionStatusOnly(t *testing.T) {
	if code, ok := commandExit("open", fmt.Errorf("open box: %w", &core.SessionEnded{Code: 3})); !ok || code != 3 {
		t.Fatalf("a Box session ending with 3: %d, %v", code, ok)
	}
	qemuFailed := fmt.Errorf("start vm: %w", exec.Command("false").Run())
	if _, ok := commandExit("open", qemuFailed); ok {
		t.Fatal("a helper failing under open must be reported, not passed through")
	}
}
