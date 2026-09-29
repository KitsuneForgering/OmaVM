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

var accentFold = func() map[rune]rune {
	fold := map[rune]rune{}
	for base, accented := range map[rune]string{
		'a': "áàâãäå", 'c': "ç", 'e': "éèêë", 'i': "íìîï",
		'o': "óòôõö", 'u': "úùûü", 'n': "ñ", 'y': "ýÿ",
	} {
		for _, r := range accented {
			fold[r] = base
		}
	}
	return fold
}()

func boxName(env core.Environment) string {
	var name strings.Builder
	name.WriteString("omavm-")
	for _, r := range strings.ToLower(env.Name) {
		// Podman only accepts [a-zA-Z0-9][a-zA-Z0-9_.-]*: accented Latin
		// letters keep their base letter so the name stays readable, and
		// anything else non-ASCII becomes a separator. The ID suffix below
		// is what keeps names unique.
		if base, ok := accentFold[r]; ok {
			r = base
		}
		if r < 0x80 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-') {
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
	if err := b.requireContainer(ctx, env); err != nil {
		return err
	}
	b.applyTravelMode(ctx, env)
	return b.run(ctx, "enter", "--no-tty", "--name", boxName(env), "--", "true")
}

func (b *Backend) Open(ctx context.Context, env core.Environment) error {
	if err := b.requireContainer(ctx, env); err != nil {
		return err
	}
	b.applyTravelMode(ctx, env)
	return b.runInteractive(ctx, "enter", "--name", boxName(env))
}

func (b *Backend) Stop(ctx context.Context, env core.Environment) error {
	if _, found, err := b.find(ctx, env); err == nil && !found {
		return nil // nothing left to stop
	}
	return b.run(ctx, "stop", "--yes", boxName(env))
}

// find looks the Box's container up in `distrobox list`, returning its raw
// status column.
func (b *Backend) find(ctx context.Context, env core.Environment) (raw string, found bool, err error) {
	out, err := b.output(ctx, "list", "--no-color")
	if err != nil {
		return "", false, err
	}
	for _, line := range strings.Split(out, "\n") {
		columns := strings.Split(line, "|")
		if len(columns) < 4 || strings.TrimSpace(columns[1]) != boxName(env) {
			continue
		}
		return strings.TrimSpace(columns[2]), true, nil
	}
	return "", false, nil
}

func missingContainer(env core.Environment) string {
	return fmt.Sprintf("the container of Box %s no longer exists (it was removed outside OmaVM); delete this Box and create it again", env.Name)
}

// requireContainer guards every `distrobox enter`: for a missing container,
// enter offers to create one out of Distrobox's own default image and,
// without a terminal, accepts by itself — silently replacing the Box's
// image (an Ubuntu Box would come back as a Fedora toolbox).
func (b *Backend) requireContainer(ctx context.Context, env core.Environment) error {
	_, found, err := b.find(ctx, env)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %s", core.ErrNotFound, missingContainer(env))
	}
	return nil
}

func (b *Backend) Status(ctx context.Context, env core.Environment) (core.Status, error) {
	raw, found, err := b.find(ctx, env)
	if err != nil {
		return core.Status{}, err
	}
	if !found {
		return core.Status{State: core.StateError, Detail: missingContainer(env)}, nil
	}
	state := core.StateStopped
	if strings.HasPrefix(strings.ToLower(raw), "up") || strings.Contains(strings.ToLower(raw), "running") {
		state = core.StateRunning
	}
	return core.Status{State: state, Detail: raw}, nil
}

func (b *Backend) Exec(ctx context.Context, env core.Environment, args []string) error {
	if err := b.requireContainer(ctx, env); err != nil {
		return err
	}
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
