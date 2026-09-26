// Package qemu adapts QEMU/KVM to the core.Backend interface for
// Machine environments: environments with an independent kernel. It
// shells out to qemu-img/qemu-system-x86_64 directly rather than through
// libvirt, keeping the dependency surface small for Phase 1.
//
// Every Machine gets its own state directory holding its disk image,
// pidfile, and QMP/VNC unix sockets — no infrastructure detail leaks
// past this package.
package qemu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

const defaultDiskSize = "20G"

// Backend implements core.Backend for Machine environments via
// qemu-img and qemu-system-x86_64. It assumes KVM is available
// (/dev/kvm); Machines are always hardware-accelerated VMs, never a
// software-emulated fallback silently substituted for it.
type Backend struct {
	stateDir string
}

// New roots the backend under OmaVM's state directory.
func New() (*Backend, error) {
	base, err := core.StateDir()
	if err != nil {
		return nil, err
	}
	return &Backend{stateDir: filepath.Join(base, "machines")}, nil
}

func (b *Backend) Name() string { return "qemu" }

func (b *Backend) dir(name string) string      { return filepath.Join(b.stateDir, name) }
func (b *Backend) diskPath(name string) string { return filepath.Join(b.dir(name), "disk.qcow2") }
func (b *Backend) pidPath(name string) string  { return filepath.Join(b.dir(name), "qemu.pid") }
func (b *Backend) qmpPath(name string) string  { return filepath.Join(b.dir(name), "qmp.sock") }
func (b *Backend) vncPath(name string) string  { return filepath.Join(b.dir(name), "vnc.sock") }

func (b *Backend) Create(ctx context.Context, env core.Environment) error {
	if err := os.MkdirAll(b.dir(env.Name), 0o755); err != nil {
		return fmt.Errorf("create machine state dir: %w", err)
	}
	disk := b.diskPath(env.Name)
	if _, err := os.Stat(disk); err == nil {
		return nil // idempotent: disk already provisioned
	}
	out, err := runOutput(ctx, "qemu-img", "create", "-f", "qcow2", disk, defaultDiskSize)
	if err != nil {
		return fmt.Errorf("qemu-img create: %w: %s", err, out)
	}
	return nil
}

func (b *Backend) Start(ctx context.Context, env core.Environment) error {
	running, err := b.isRunning(env.Name)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	args := []string{
		"-name", env.Name,
		"-m", "2048",
		"-smp", "2",
		"-enable-kvm",
		"-drive", fmt.Sprintf("file=%s,if=virtio,format=qcow2", b.diskPath(env.Name)),
		"-vnc", "unix:" + b.vncPath(env.Name),
		"-qmp", "unix:" + b.qmpPath(env.Name) + ",server,nowait",
		"-pidfile", b.pidPath(env.Name),
		"-display", "none",
		"-daemonize",
	}
	if env.Image != "" {
		args = append(args, "-cdrom", env.Image, "-boot", "d")
	}

	out, err := runOutput(ctx, "qemu-system-x86_64", args...)
	if err != nil {
		return fmt.Errorf("qemu-system-x86_64: %w: %s", err, out)
	}
	return nil
}

// Open ensures the Machine is running. There is no built-in display
// attach yet for an already-running Machine beyond the VNC unix socket
// reported by Status; a first-class viewer is future work (see
// Blend Mode / Omarchy Integration in CLAUDE.md), not hidden but not
// built prematurely either.
func (b *Backend) Open(ctx context.Context, env core.Environment) error {
	return b.Start(ctx, env)
}

func (b *Backend) Stop(ctx context.Context, env core.Environment) error {
	running, err := b.isRunning(env.Name)
	if err != nil {
		return err
	}
	if !running {
		return nil
	}

	if err := qmpCommand(b.qmpPath(env.Name), "quit"); err == nil {
		return nil
	}

	pid, err := b.readPID(env.Name)
	if err != nil {
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find machine process: %w", err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop machine process: %w", err)
	}
	return nil
}

func (b *Backend) Status(ctx context.Context, env core.Environment) (core.Status, error) {
	running, err := b.isRunning(env.Name)
	if err != nil {
		return core.Status{}, err
	}
	if running {
		return core.Status{State: core.StateRunning, Detail: "vnc: unix:" + b.vncPath(env.Name)}, nil
	}
	return core.Status{State: core.StateStopped}, nil
}

// Exec has no guest command channel yet (no omavm-guest agent, see
// CLAUDE.md): Machines don't get Exec in Phase 1, and this must fail
// loudly rather than pretend to run something inside the guest.
func (b *Backend) Exec(ctx context.Context, env core.Environment, args []string) error {
	return fmt.Errorf("%w: machine %s has no guest command channel (omavm-guest not implemented)", core.ErrUnsupported, env.Name)
}

func (b *Backend) Remove(ctx context.Context, env core.Environment) error {
	if err := b.Stop(ctx, env); err != nil {
		return err
	}
	if err := os.RemoveAll(b.dir(env.Name)); err != nil {
		return fmt.Errorf("remove machine state dir: %w", err)
	}
	return nil
}

func (b *Backend) readPID(name string) (int, error) {
	data, err := os.ReadFile(b.pidPath(name))
	if err != nil {
		return 0, fmt.Errorf("read pidfile: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse pidfile: %w", err)
	}
	return pid, nil
}

func (b *Backend) isRunning(name string) (bool, error) {
	pid, err := b.readPID(name)
	if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, nil
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false, nil
	}
	return true, nil
}

func runOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
