package qemu

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

func TestLinkCreatesAndUpdatesSymlink(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "guest", Kind: core.Machine}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatalf("Create: %v", err)
	}

	link, err := b.Link(context.Background(), env, "blue")
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("expected a symlink at %s: %v", link, err)
	}
	if target != b.diskPath(env.Name) {
		t.Fatalf("expected link to point at %s, got %s", b.diskPath(env.Name), target)
	}

	// Re-linking (e.g. a color change) must replace the stale symlink,
	// not fail or duplicate it.
	if _, err := b.Link(context.Background(), env, "red"); err != nil {
		t.Fatalf("re-Link: %v", err)
	}

	if err := b.Unlink(context.Background(), env); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("expected link to be removed, stat err: %v", err)
	}
	// Unlink must be idempotent.
	if err := b.Unlink(context.Background(), env); err != nil {
		t.Fatalf("second Unlink: %v", err)
	}
}

func TestLinkPathIsUnderHomeOmaVM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "guest", Kind: core.Machine}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	link, err := b.Link(context.Background(), env, "")
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	want := filepath.Join(home, "OmaVM", "guest")
	if link != want {
		t.Fatalf("expected link at %s, got %s", want, link)
	}
}
