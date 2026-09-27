// Package distrobox adapts Distrobox as OmaVM's integrated Development Box backend.
package distrobox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

type Backend struct{}

func New() *Backend           { return &Backend{} }
func (*Backend) Name() string { return "distrobox" }

func boxName(env core.Environment) string {
	var name strings.Builder
	name.WriteString("omavm-")
	for _, r := range strings.ToLower(env.Name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			name.WriteRune(r)
		} else if name.Len() == 0 || !strings.HasSuffix(name.String(), "-") {
			name.WriteByte('-')
		}
	}
	clean := strings.TrimRight(name.String(), "-")
	id := strings.ReplaceAll(env.ID, "-", "")
	if len(id) > 8 {
		id = id[:8]
	}
	if id == "" {
		return clean
	}
	return clean + "-" + id
}

func (b *Backend) Create(ctx context.Context, env core.Environment) error {
	return b.run(ctx, "create", "--yes", "--name", boxName(env), "--image", env.Image)
}

func (b *Backend) Start(ctx context.Context, env core.Environment) error {
	return b.run(ctx, "enter", "--no-tty", "--name", boxName(env), "--", "true")
}

func (b *Backend) Open(ctx context.Context, env core.Environment) error {
	return b.runInteractive(ctx, "enter", "--name", boxName(env))
}

func (b *Backend) Stop(ctx context.Context, env core.Environment) error {
	return b.run(ctx, "stop", "--yes", boxName(env))
}

func (b *Backend) Status(ctx context.Context, env core.Environment) (core.Status, error) {
	out, err := b.output(ctx, "list", "--no-color")
	if err != nil {
		return core.Status{}, err
	}
	for _, line := range strings.Split(out, "\n") {
		columns := strings.Split(line, "|")
		if len(columns) < 4 || strings.TrimSpace(columns[1]) != boxName(env) {
			continue
		}
		raw := strings.TrimSpace(columns[2])
		state := core.StateStopped
		if strings.HasPrefix(strings.ToLower(raw), "up") || strings.Contains(strings.ToLower(raw), "running") {
			state = core.StateRunning
		}
		return core.Status{State: state, Detail: raw}, nil
	}
	return core.Status{}, fmt.Errorf("%w: distrobox %s", core.ErrNotFound, env.Name)
}

func (b *Backend) Exec(ctx context.Context, env core.Environment, args []string) error {
	full := append([]string{"enter", "--no-tty", "--name", boxName(env), "--"}, args...)
	return b.runInteractive(ctx, full...)
}

func (b *Backend) Remove(ctx context.Context, env core.Environment) error {
	return b.run(ctx, "rm", "--force", boxName(env))
}

func (b *Backend) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	path, err := exec.LookPath("distrobox")
	if err != nil {
		return nil, fmt.Errorf("distrobox is required for Development Boxes (install it with: sudo pacman -S distrobox): %w", err)
	}
	return exec.CommandContext(ctx, path, args...), nil
}

func (b *Backend) output(ctx context.Context, args ...string) (string, error) {
	cmd, err := b.command(ctx, args...)
	if err != nil {
		return "", err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("distrobox %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (b *Backend) run(ctx context.Context, args ...string) error {
	_, err := b.output(ctx, args...)
	return err
}

func (b *Backend) runInteractive(ctx context.Context, args ...string) error {
	cmd, err := b.command(ctx, args...)
	if err != nil {
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("distrobox %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
