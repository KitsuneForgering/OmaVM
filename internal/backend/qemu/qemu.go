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
	"net"
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
func (b *Backend) vncDisplayPath(name string) string {
	return filepath.Join(b.dir(name), "vnc-display")
}
func (b *Backend) previewPath(name string) string { return filepath.Join(b.dir(name), "preview.ppm") }

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

	display, err := findFreeVNCDisplay()
	if err != nil {
		return err
	}

	args := []string{
		"-name", env.Name,
		"-m", "2048",
		"-smp", "2",
		"-enable-kvm",
		"-drive", fmt.Sprintf("file=%s,if=virtio,format=qcow2", b.diskPath(env.Name)),
		"-vnc", fmt.Sprintf("127.0.0.1:%d", display),
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
	if err := os.WriteFile(b.vncDisplayPath(env.Name), []byte(strconv.Itoa(display)), 0o644); err != nil {
		return fmt.Errorf("record vnc display: %w", err)
	}
	return nil
}

// Open ensures the Machine is running, then launches a VNC viewer so
// its graphical userspace is actually visible — not left as a socket
// nobody's looking at. Requires a VNC client on PATH; without one, this
// fails loudly with the connection endpoint and what to install,
// instead of silently no-op'ing (Security/UX principle: never hide a
// capability gap).
func (b *Backend) Open(ctx context.Context, env core.Environment) error {
	if err := b.Start(ctx, env); err != nil {
		return err
	}
	addr, err := b.vncAddress(env.Name)
	if err != nil {
		return err
	}

	viewer, viewerArgs, err := vncViewerCommand(addr)
	if err != nil {
		return fmt.Errorf("%w: no VNC client found on PATH (install virt-viewer or tigervnc) — connect to vnc://%s yourself", core.ErrUnsupported, addr)
	}

	// Detached on purpose: the viewer is a GUI the user drives for a
	// while, not something that should die when this call's context
	// ends (same reasoning as the GUI's own openInTerminal).
	cmd := exec.CommandContext(context.Background(), viewer, viewerArgs...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch VNC viewer: %w", err)
	}
	return nil
}

// findFreeVNCDisplay probes 127.0.0.1:5900+N upward for the first port
// that binds. QEMU's -vnc option takes a display number (an offset from
// 5900), not a raw port. The probe-then-close-then-let-qemu-bind
// sequence has a small unavoidable race — acceptable for a local dev
// tool; a stolen port just surfaces as a clear qemu-system-x86_64 error
// to retry.
func findFreeVNCDisplay() (int, error) {
	for display := 0; display < 100; display++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 5900+display))
		if err != nil {
			continue
		}
		l.Close()
		return display, nil
	}
	return 0, errors.New("no free VNC display found in range 5900-5999")
}

// vncViewerCommand picks the first available VNC client on PATH.
// remote-viewer (virt-viewer) understands a vnc:// URI directly; plain
// VNC viewers take a bare host:port.
func vncViewerCommand(addr string) (string, []string, error) {
	if path, err := exec.LookPath("remote-viewer"); err == nil {
		return path, []string{"vnc://" + addr}, nil
	}
	if path, err := exec.LookPath("vncviewer"); err == nil {
		return path, []string{addr}, nil
	}
	if path, err := exec.LookPath("gvncviewer"); err == nil {
		return path, []string{addr}, nil
	}
	return "", nil, errors.New("no VNC client on PATH")
}

func (b *Backend) vncAddress(name string) (string, error) {
	data, err := os.ReadFile(b.vncDisplayPath(name))
	if err != nil {
		return "", fmt.Errorf("read vnc display: %w", err)
	}
	display, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return "", fmt.Errorf("parse vnc display: %w", err)
	}
	return fmt.Sprintf("127.0.0.1:%d", 5900+display), nil
}

// Preview captures a screenshot of a running Machine's graphical
// userspace via QMP screendump — no VNC/RFB client implementation
// needed, since QEMU writes the framebuffer straight to a local file.
// Returns a PPM image path; gdk-pixbuf loads PNM natively, so the GUI
// can hand this straight to a Picture/Image widget.
func (b *Backend) Preview(ctx context.Context, env core.Environment) (string, error) {
	running, err := b.isRunning(env.Name)
	if err != nil {
		return "", err
	}
	if !running {
		return "", fmt.Errorf("%w: machine is not running", core.ErrUnsupported)
	}

	dst := b.previewPath(env.Name)
	if err := qmpScreendump(b.qmpPath(env.Name), dst); err != nil {
		return "", fmt.Errorf("screendump: %w", err)
	}
	return dst, nil
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
	if !running {
		return core.Status{State: core.StateStopped}, nil
	}
	detail := "vnc: unreachable"
	if addr, err := b.vncAddress(env.Name); err == nil {
		detail = "vnc: " + addr
	}
	return core.Status{State: core.StateRunning, Detail: detail}, nil
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
