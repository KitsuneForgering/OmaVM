// Package container is OmaVM's own Box engine: a minimal adapter built
// directly on Podman (preferred) or Docker, not on the external
// `distrobox` binary. Per CLAUDE.md's Backend Rules, it stays
// deliberately small — create/start/stop/exec/remove plus a home
// directory mount and host networking — and never reimplements the
// container engine itself (image storage, runc/OCI): that stays Podman
// or Docker's job.
package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// containerPrefix namespaces containers this engine manages so it never
// collides with or accidentally touches unrelated containers on the
// host.
const containerPrefix = "omavm-box-"

// Backend implements core.Backend for Box environments via a
// container engine CLI (podman or docker — their CLIs are
// compatible for the subset of commands used here).
type Backend struct {
	runtime string
}

// New detects the available container engine, preferring podman. If
// neither is found on PATH, it still returns a Backend defaulting to
// "podman": operations then fail with a clear "executable not found"
// error from exec, the same way a missing engine has always surfaced.
func New() *Backend {
	if _, err := exec.LookPath("podman"); err == nil {
		return &Backend{runtime: "podman"}
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return &Backend{runtime: "docker"}
	}
	return &Backend{runtime: "podman"}
}

func (b *Backend) Name() string { return b.runtime }

func containerName(env core.Environment) string {
	return containerPrefix + env.Name
}

func (b *Backend) Create(ctx context.Context, env core.Environment) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	args := []string{
		"create",
		"--name", containerName(env),
		"--hostname", env.Name,
		"--volume", home + ":" + home,
		"--network", "host",
		"--entrypoint", "sleep",
		env.Image,
		"infinity",
	}
	return b.run(ctx, args...)
}

func (b *Backend) Start(ctx context.Context, env core.Environment) error {
	return b.run(ctx, "start", containerName(env))
}

// Open attaches an interactive shell, starting the container first if
// needed.
func (b *Backend) Open(ctx context.Context, env core.Environment) error {
	if err := b.Start(ctx, env); err != nil {
		return err
	}
	return b.runInteractive(ctx, "exec", "-it", containerName(env), shellFor(env))
}

func (b *Backend) Stop(ctx context.Context, env core.Environment) error {
	return b.run(ctx, "stop", containerName(env))
}

func (b *Backend) Status(ctx context.Context, env core.Environment) (core.Status, error) {
	cmd := exec.CommandContext(ctx, b.runtime, "inspect", "--format", "{{.State.Status}}", containerName(env))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return core.Status{}, fmt.Errorf("%w: %s has no container named %s: %s", core.ErrNotFound, b.runtime, env.Name, strings.TrimSpace(string(out)))
	}
	rawStatus := strings.TrimSpace(string(out))
	state := core.StateStopped
	if rawStatus == "running" {
		state = core.StateRunning
	}
	return core.Status{State: state, Detail: rawStatus}, nil
}

func (b *Backend) Exec(ctx context.Context, env core.Environment, args []string) error {
	full := append([]string{"exec", containerName(env)}, args...)
	return b.runInteractive(ctx, full...)
}

func (b *Backend) Remove(ctx context.Context, env core.Environment) error {
	return b.run(ctx, "rm", "--force", containerName(env))
}

func shellFor(env core.Environment) string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}

func (b *Backend) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, b.runtime, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", b.runtime, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (b *Backend) runInteractive(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, b.runtime, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", b.runtime, strings.Join(args, " "), err)
	}
	return nil
}
