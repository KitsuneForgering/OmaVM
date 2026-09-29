package distrobox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
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

// Podman only accepts container names matching [a-zA-Z0-9][a-zA-Z0-9_.-]*
// (checked against podman on 2026-09-28), while environment names may use
// accents and any script. Every derived name must be valid, and existing
// ASCII names must map exactly as before so current Boxes keep working.
func TestBoxNameIsAValidContainerName(t *testing.T) {
	valid := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	const id = "abcd1234ef"
	tests := map[string]string{
		"radic":           "omavm-radic-abcd1234",
		"My Box":          "omavm-my-box-abcd1234",
		"Café":            "omavm-cafe-abcd1234",
		"Projeto Ação":    "omavm-projeto-acao-abcd1234",
		"Pão de Queijo ü": "omavm-pao-de-queijo-u-abcd1234",
		"日本":              "omavm-abcd1234",
		"----":            "omavm-abcd1234",
	}
	for name, want := range tests {
		got := boxName(core.Environment{Name: name, ID: id})
		if !valid.MatchString(got) {
			t.Errorf("boxName(%q) = %q is not a valid container name", name, got)
		}
		if got != want {
			t.Errorf("boxName(%q) = %q, want %q", name, got, want)
		}
	}
}

// When a Box's container is removed outside OmaVM (podman rm, a podman
// system reset), `distrobox enter` offers to create it "out of image" its
// own default — and without a terminal it answers yes by itself. Start
// would silently turn an Ubuntu Box into a fresh Fedora toolbox. OmaVM
// must refuse instead, and say what happened.
func TestMissingContainerIsNeverRecreatedByEnter(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls")
	writeDistrobox(t, bin, `echo "$1" >> "$OMAVM_TEST_LOG"
if [ "$1" = list ]; then printf 'ID | NAME | STATUS | IMAGE\n1 | omavm-other-99999999 | Up | fedora\n'; fi`)
	t.Setenv("PATH", bin)
	t.Setenv("OMAVM_TEST_LOG", log)
	env := core.Environment{ID: "12345678", Name: "ubuntu", Image: "ubuntu:latest"}
	b := New()

	for name, op := range map[string]func() error{
		"start": func() error { return b.Start(context.Background(), env) },
		"exec":  func() error { return b.Exec(context.Background(), env, []string{"true"}) },
		"open":  func() error { return b.Open(context.Background(), env) },
	} {
		err := op()
		if !errors.Is(err, core.ErrNotFound) || !strings.Contains(err.Error(), "create it again") {
			t.Errorf("%s on a missing container = %v, want ErrNotFound saying to create it again", name, err)
		}
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "enter") {
		t.Fatalf("distrobox enter was called on a missing container:\n%s", calls)
	}

	status, err := b.Status(context.Background(), env)
	if err != nil || status.State != core.StateError || !strings.Contains(status.Detail, "no longer exists") {
		t.Fatalf("Status of a missing container = %#v, %v; want an error state explaining it", status, err)
	}
	if err := b.Stop(context.Background(), env); err != nil {
		t.Fatalf("Stop of a missing container must be a no-op: %v", err)
	}
}
