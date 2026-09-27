package distrobox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

func TestCreateUsesStableUniqueDistroboxName(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "args")
	writeDistrobox(t, bin, `printf '%s\n' "$@" > "$OMAVM_TEST_LOG"`)
	t.Setenv("PATH", bin)
	t.Setenv("OMAVM_TEST_LOG", log)

	env := core.Environment{ID: "12345678-abcd", Name: "Fast Box!", Image: "fedora:latest"}
	if err := New().Create(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "create\n--yes\n--name\nomavm-fast-box-12345678\n--image\nfedora:latest\n"
	if string(got) != want {
		t.Fatalf("unexpected arguments:\n%s\nwant:\n%s", got, want)
	}
}

func TestStatusParsesDistroboxList(t *testing.T) {
	bin := t.TempDir()
	writeDistrobox(t, bin, `printf 'ID | NAME | STATUS | IMAGE\n1 | omavm-fast-12345678 | Up 2 minutes | fedora:latest\n'`)
	t.Setenv("PATH", bin)

	status, err := New().Status(context.Background(), core.Environment{ID: "12345678", Name: "fast"})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != core.StateRunning || status.Detail != "Up 2 minutes" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestMissingDistroboxReturnsInstallHint(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := New().Create(context.Background(), core.Environment{Name: "fast", Image: "fedora"})
	if err == nil || !strings.Contains(err.Error(), "sudo pacman -S distrobox") {
		t.Fatalf("expected install hint, got %v", err)
	}
}

func writeDistrobox(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, "distrobox")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
