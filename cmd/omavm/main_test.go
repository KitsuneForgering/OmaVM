package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
