package qemu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// hostLinkDir is ~/OmaVM, the directory OmaVM exposes Machine disks
// under so an Environment has a real, browsable presence on the host
// filesystem — the same role a .pvm bundle plays for Parallels, and a
// prerequisite for a color tag to have anything to attach to.
func hostLinkDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "OmaVM"), nil
}

func (b *Backend) hostLinkPath(name string) (string, error) {
	dir, err := hostLinkDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// Link ensures ~/OmaVM/<name> exists as a symlink to the Machine's disk
// image, then applies color as a best-effort user.xdg.tags xattr on it.
// Best-effort because whether any given file manager actually reads that
// xattr (Dolphin/Baloo do; many others, including what Omarchy ships by
// default, may not) is outside OmaVM's control — never promise a
// guarantee that isn't real (Security Model). A missing setfattr binary
// or a failed xattr write does not fail Link: the visible link itself is
// the real, guaranteed part.
func (b *Backend) Link(ctx context.Context, env core.Environment, color string) (string, error) {
	dir, err := hostLinkDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	link, err := b.hostLinkPath(env.Name)
	if err != nil {
		return "", err
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove stale link: %w", err)
	}
	if err := os.Symlink(b.diskPath(env.Name), link); err != nil {
		return "", fmt.Errorf("link %s: %w", link, err)
	}
	if color != "" {
		if path, lookErr := exec.LookPath("setfattr"); lookErr == nil {
			_ = exec.CommandContext(ctx, path, "-n", "user.xdg.tags", "-v", color, link).Run()
		}
	}
	return link, nil
}

func (b *Backend) Unlink(ctx context.Context, env core.Environment) error {
	link, err := b.hostLinkPath(env.Name)
	if err != nil {
		return err
	}
	if err := os.Remove(link); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", link, err)
	}
	return nil
}
