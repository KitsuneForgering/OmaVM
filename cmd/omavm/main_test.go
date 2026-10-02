package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KitsuneSemCalda/OmaVM/internal/backend/machine/qemu"
)

func TestReadOnlyCommands(t *testing.T) {
	tests := []struct {
		cmd  string
		args []string
		want bool
	}{
		{"list", []string{"--json"}, true},
		{"status", []string{"vm", "--json"}, true},
		{"integration", []string{"vm"}, true},
		{"snapshot", []string{"list", "vm"}, true},
		{"snapshot", []string{"create", "vm", "--label", "x"}, false},
		{"apps", []string{"box", "--json"}, true},
		{"apps", []string{"box", "--export", "/usr/share/applications/a.desktop"}, false},
		{"apps", []string{"box", "--unexport=/x.desktop"}, false},
		{"settings", []string{"vm"}, true},
		{"settings", []string{"vm", "--cpus", "4"}, false},
		{"start", []string{"vm"}, false},
		{"rm", []string{"vm"}, false},
	}
	for _, tt := range tests {
		if got := readOnly(tt.cmd, tt.args); got != tt.want {
			t.Errorf("readOnly(%q, %q) = %t, want %t", tt.cmd, tt.args, got, tt.want)
		}
	}
}

// Regression: a Machine opened from the app launcher failed silently.
func TestNotifyErrorSendsDesktopNotification(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "notify-send"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	notifyError(errors.New("start kernels: installation media missing"))
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "start kernels: installation media missing") {
		t.Fatalf("notification did not carry the error: %s", got)
	}
}

func TestNotifyErrorWithoutNotifySendIsSilent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	notifyError(errors.New("x"))
}

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// The JSON the CLI prints is a contract: the GUI, the Omarchy bar plugin
// and agents parse it. A change to it must be a decision, visible as a
// golden file diff, never a side effect of renaming a Go field.
func TestJSONContract(t *testing.T) {
	withTestRegistry(t)

	for golden, args := range map[string][]string{
		"list.golden.json":          {"list", "--json"},
		"list-status.golden.json":   {"list", "--status", "--json"},
		"status.golden.json":        {"status", "Desktop Fedora", "--json"},
		"snapshot-list.golden.json": {"snapshot", "list", "Desktop Fedora", "--json"},
		"settings.golden.json":      {"settings", "dev", "--json"},
	} {
		t.Run(golden, func(t *testing.T) {
			got := captureStdout(t, func() error { return run(args) })
			path := filepath.Join("testdata", golden)
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test ./cmd/omavm -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("omavm %s changed its JSON:\n got: %s\nwant: %s", strings.Join(args, " "), got, want)
			}
		})
	}
}

// withTestRegistry points OmaVM at a copy of testdata/environments.json,
// with no container engine and no QEMU: every status is decided without
// touching the host.
func withTestRegistry(t *testing.T) {
	t.Helper()
	registry, err := os.ReadFile(filepath.Join("testdata", "environments.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	if err := os.MkdirAll(filepath.Join(state, "omavm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "omavm", "environments.json"), registry, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DBX_CONTAINER_MANAGER", "")
	// Plenty of space, whatever the test machine has: otherwise a full
	// /tmp adds a low-space warning to every Machine's status.
	saved := qemu.FreeSpace
	qemu.FreeSpace = func(string) (uint64, bool) { return 1 << 40, true }
	t.Cleanup(func() { qemu.FreeSpace = saved })
}

// Regression: --shared-writable=false made the shared folder writable,
// the opposite of what it says.
func TestSharedFolderModeFlags(t *testing.T) {
	withTestRegistry(t)
	for _, tc := range []struct {
		flag     string
		readOnly bool
	}{
		{"--shared-read-only", true},
		{"--shared-writable", false},
		{"--shared-writable=false", true},
		{"--shared-read-only=false", false},
		{"--shared-writable=true", false},
	} {
		out := captureStdout(t, func() error { return run([]string{"settings", "Desktop Fedora", tc.flag, "--json"}) })
		var settings struct {
			ReadOnly bool `json:"shared_read_only"`
		}
		if err := json.Unmarshal(out, &settings); err != nil {
			t.Fatal(err)
		}
		if settings.ReadOnly != tc.readOnly {
			t.Errorf("%s: read-only = %t, want %t", tc.flag, settings.ReadOnly, tc.readOnly)
		}
	}
}

func captureStdout(t *testing.T, fn func() error) []byte {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	runErr := fn()
	os.Stdout = stdout
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if runErr != nil {
		t.Fatal(runErr)
	}
	return out
}
