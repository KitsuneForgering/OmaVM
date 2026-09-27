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
	"time"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

const defaultDiskSize = "20G"
const gracefulShutdownTimeout = 10 * time.Second

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
func (b *Backend) qgaPath(name string) string  { return filepath.Join(b.dir(name), "qga.sock") }
func (b *Backend) virtiofsPath(name string) string {
	return filepath.Join(b.dir(name), "virtiofs.sock")
}
func (b *Backend) virtiofsPIDPath(name string) string {
	return filepath.Join(b.dir(name), "virtiofs.pid")
}
func (b *Backend) vncPath(name string) string     { return filepath.Join(b.dir(name), "vnc.sock") }
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

	for _, socket := range []string{b.vncPath(env.Name), b.qgaPath(env.Name)} {
		if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale socket: %w", err)
		}
	}
	settings := env.EffectiveSettings()
	virtiofsRunning := false
	if settings.SharedPath != "" {
		if err := b.startVirtiofs(ctx, env.Name, settings); err != nil {
			return err
		}
		virtiofsRunning = true
	}
	vnc := b.vncPath(env.Name)

	args := []string{
		"-name", env.Name,
		"-m", strconv.Itoa(settings.MemoryMiB),
		"-smp", strconv.Itoa(settings.CPUs),
		"-enable-kvm",
		"-drive", fmt.Sprintf("file=%s,if=virtio,format=qcow2", b.diskPath(env.Name)),
		"-device", "virtio-vga-gl",
		// egl-headless keeps virtio-vga-gl's GPU acceleration while
		// headless; VNC is the display consumer QEMU pairs it with
		// (no local GTK/SDL window, no SPICE).
		"-display", "egl-headless",
		"-vnc", "unix:" + vnc,
		"-device", "virtio-serial-pci",
		"-chardev", "socket,path=" + b.qgaPath(env.Name) + ",server=on,wait=off,id=qga0",
		"-device", "virtserialport,chardev=qga0,name=org.qemu.guest_agent.0",
		"-audiodev", "pipewire,id=audio0",
		"-device", "virtio-sound-pci,audiodev=audio0",
		"-qmp", "unix:" + b.qmpPath(env.Name) + ",server,nowait",
		"-pidfile", b.pidPath(env.Name),
		"-daemonize",
	}
	if virtiofsRunning {
		args = append(args,
			"-object", fmt.Sprintf("memory-backend-memfd,id=mem,size=%dM,share=on", settings.MemoryMiB),
			"-machine", "memory-backend=mem",
			"-chardev", "socket,id=virtiofs,path="+b.virtiofsPath(env.Name),
			"-device", "vhost-user-fs-pci,chardev=virtiofs,tag=omavm-share")
	}
	if env.Image != "" {
		args = append(args, "-cdrom", env.Image, "-boot", "d")
	}

	out, err := runOutput(ctx, "qemu-system-x86_64", args...)
	if err != nil {
		if virtiofsRunning {
			_ = b.stopVirtiofs(env.Name)
		}
		return fmt.Errorf("qemu-system-x86_64: %w: %s", err, out)
	}
	return nil
}

func virtiofsdPath() (string, error) {
	if path, err := exec.LookPath("virtiofsd"); err == nil {
		return path, nil
	}
	if info, err := os.Stat("/usr/lib/virtiofsd"); err == nil && info.Mode().Perm()&0o111 != 0 {
		return "/usr/lib/virtiofsd", nil
	}
	return "", fmt.Errorf("virtiofsd not found (install the virtiofsd package)")
}

func (b *Backend) startVirtiofs(ctx context.Context, name string, settings core.EnvironmentSettings) error {
	path, err := virtiofsdPath()
	if err != nil {
		return fmt.Errorf("shared folder: %w", err)
	}
	if err := os.Remove(b.virtiofsPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale virtiofs socket: %w", err)
	}
	args := []string{"--shared-dir", settings.SharedPath, "--socket-path", b.virtiofsPath(name), "--sandbox", "namespace", "--tag", "omavm-share"}
	if settings.SharedReadOnly {
		args = append(args, "--readonly")
	}
	cmd := exec.CommandContext(context.Background(), path, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start virtiofsd: %w", err)
	}
	if err := os.WriteFile(b.virtiofsPIDPath(name), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("record virtiofsd pid: %w", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(b.virtiofsPath(name)); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			_ = b.stopVirtiofs(name)
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	_ = b.stopVirtiofs(name)
	return fmt.Errorf("virtiofsd did not create its socket")
}

func (b *Backend) stopVirtiofs(name string) error {
	data, err := os.ReadFile(b.virtiofsPIDPath(name))
	if err == nil {
		if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil {
			if proc, findErr := os.FindProcess(pid); findErr == nil {
				_ = proc.Signal(syscall.SIGTERM)
			}
		}
	}
	_ = os.Remove(b.virtiofsPIDPath(name))
	_ = os.Remove(b.virtiofsPath(name))
	return nil
}

// Open ensures the Machine is running, then launches omavm-gui in its
// native VNC viewer mode so the graphical userspace is actually visible —
// not left as a socket nobody's looking at. Requires omavm-gui to be
// installed; without it, this fails loudly with what to install, instead
// of silently no-op'ing (Security/UX principle: never hide a capability
// gap).
func (b *Backend) Open(ctx context.Context, env core.Environment) error {
	if err := b.Start(ctx, env); err != nil {
		return err
	}
	viewer, err := guiBinaryPath()
	if err != nil {
		return fmt.Errorf("%w: omavm-gui not found (install OmaVM's GUI to view a Machine's display)", core.ErrUnsupported)
	}

	// Detached on purpose: the viewer is a GUI the user drives for a
	// while, not something that should die when this call's context
	// ends (same reasoning as the GUI's own openInTerminal).
	cmd := exec.CommandContext(context.Background(), viewer,
		"--viewer", b.vncPath(env.Name),
		"--title", env.Name+" — OmaVM")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch VNC viewer: %w", err)
	}
	return nil
}

// guiBinaryPath locates omavm-gui the same way the GUI itself locates the
// omavm CLI (gui/backend.cpp's cliPath): next to this process's own binary
// first, falling back to PATH.
func guiBinaryPath() (string, error) {
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "omavm-gui")
		if info, statErr := os.Stat(sibling); statErr == nil && !info.IsDir() {
			return sibling, nil
		}
	}
	return exec.LookPath("omavm-gui")
}

// Preview captures a screenshot of a running Machine's graphical
// userspace via QMP screendump — no SPICE client implementation
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
		_ = b.stopVirtiofs(env.Name)
		return nil
	}
	if status, err := qmpStatus(b.qmpPath(env.Name)); err == nil && (status == "paused" || status == "suspended") {
		if err := qmpCommand(b.qmpPath(env.Name), "cont"); err != nil {
			return fmt.Errorf("resume machine before shutdown: %w", err)
		}
	}

	// Ask the guest to shut down through ACPI first. If it does not exit in
	// time (for example, an installer has no ACPI handler), force-stop QEMU.
	if err := qmpCommand(b.qmpPath(env.Name), "system_powerdown"); err == nil {
		deadline := time.Now().Add(gracefulShutdownTimeout)
		for time.Now().Before(deadline) {
			if running, _ := b.isRunning(env.Name); !running {
				_ = b.stopVirtiofs(env.Name)
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return b.ForceStop(ctx, env)
}

func (b *Backend) Restart(ctx context.Context, env core.Environment) error {
	if running, err := b.isRunning(env.Name); err != nil {
		return err
	} else if !running {
		return b.Start(ctx, env)
	}
	status, err := qmpStatus(b.qmpPath(env.Name))
	if err != nil {
		return err
	}
	if err := qmpCommand(b.qmpPath(env.Name), "system_reset"); err != nil {
		return err
	}
	if status == "paused" || status == "suspended" {
		return qmpCommand(b.qmpPath(env.Name), "cont")
	}
	return nil
}

func (b *Backend) Pause(ctx context.Context, env core.Environment) error {
	if running, err := b.isRunning(env.Name); err != nil {
		return err
	} else if !running {
		return fmt.Errorf("%w: machine is not running", core.ErrUnsupported)
	}
	return qmpCommand(b.qmpPath(env.Name), "stop")
}

func (b *Backend) Resume(ctx context.Context, env core.Environment) error {
	if running, err := b.isRunning(env.Name); err != nil {
		return err
	} else if !running {
		return fmt.Errorf("%w: machine is not running", core.ErrUnsupported)
	}
	return qmpCommand(b.qmpPath(env.Name), "cont")
}

func (b *Backend) ForceStop(ctx context.Context, env core.Environment) error {
	if running, err := b.isRunning(env.Name); err != nil {
		return err
	} else if !running {
		_ = b.stopVirtiofs(env.Name)
		return nil
	}
	if err := qmpCommand(b.qmpPath(env.Name), "quit"); err == nil {
		_ = b.stopVirtiofs(env.Name)
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
	_ = b.stopVirtiofs(env.Name)
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
	status, err := qmpStatus(b.qmpPath(env.Name))
	if err != nil {
		return core.Status{State: core.StateUnknown, Detail: err.Error()}, nil
	}
	return statusFromQMP(status), nil
}

func (b *Backend) Integration(ctx context.Context, env core.Environment) (core.IntegrationReport, error) {
	running, err := b.isRunning(env.Name)
	if err != nil {
		return core.IntegrationReport{}, err
	}
	if !running {
		return core.IntegrationReport{GuestAgent: "stopped", Hint: "Start the Machine to check guest tools"}, nil
	}
	if err := qgaPing(ctx, b.qgaPath(env.Name)); err != nil {
		return core.IntegrationReport{GuestAgent: "unavailable", Hint: "Install and start qemu-guest-agent inside the guest"}, nil
	}
	return core.IntegrationReport{GuestAgent: "connected"}, nil
}

func statusFromQMP(status string) core.Status {
	switch status {
	case "running":
		return core.Status{State: core.StateRunning, Detail: "VNC display available"}
	case "paused", "suspended":
		return core.Status{State: core.StatePaused}
	case "prelaunch", "inmigrate", "restore-vm":
		return core.Status{State: core.StateStarting, Detail: status}
	case "shutdown":
		return core.Status{State: core.StateStopping}
	case "internal-error", "io-error", "watchdog", "guest-panicked":
		return core.Status{State: core.StateError, Detail: status}
	default:
		return core.Status{State: core.StateUnknown, Detail: status}
	}
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
