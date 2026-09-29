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
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
	"github.com/KitsuneSemCalda/OmaVM/internal/power"
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

// maxSocketPath is the usable length of a Unix socket path (sun_path is
// 108 bytes including the terminating NUL). The Machine's name is part of
// every socket path, so a long name would otherwise pass Create and only
// fail at Start with QEMU's "UNIX socket path is too long".
const maxSocketPath = 107

func (b *Backend) checkNameLength(name string) error {
	longest := b.virtiofsPath(name)
	if len(longest) <= maxSocketPath {
		return nil
	}
	limit := len(name) - (len(longest) - maxSocketPath)
	if limit < 1 {
		return core.Invalidf("the OmaVM state directory %s is too long for a Machine's sockets; set XDG_STATE_HOME to a shorter path", b.stateDir)
	}
	return core.Invalidf("machine name is too long; use at most %d bytes (accented letters count as two)", limit)
}

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
func (b *Backend) previewPath(name string) string { return filepath.Join(b.dir(name), "preview.ppm") }

func (b *Backend) Create(ctx context.Context, env core.Environment) error {
	if err := b.checkNameLength(env.Name); err != nil {
		return err
	}
	if err := os.MkdirAll(b.dir(env.Name), 0o755); err != nil {
		return fmt.Errorf("create machine state dir: %w", err)
	}
	disk := b.diskPath(env.Name)
	if _, err := os.Stat(disk); err == nil {
		return nil // idempotent: disk already provisioned
	}
	out, err := runQEMU(ctx, "qemu-img", "create", "-f", "qcow2", disk, defaultDiskSize)
	if err != nil {
		return qemuErr("qemu-img create", err, out)
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
	if err := b.checkNameLength(env.Name); err != nil {
		return err
	}

	for _, socket := range []string{b.qgaPath(env.Name), b.qmpPath(env.Name)} {
		if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale socket: %w", err)
		}
	}
	settings := env.EffectiveSettings()
	if !settings.TravelModeDisabled && env.Settings.CPUs == 0 && power.OnBattery() {
		// Travel Mode: the host is unplugged and the user hasn't pinned
		// CPUs explicitly, so trim this session's allocation — never the
		// persisted setting — instead of running full tilt on battery.
		// Real host automation, not a manual toggle (UX Principles #4/#7).
		if reduced := settings.CPUs / 2; reduced >= 1 {
			settings.CPUs = reduced
		}
		slog.Info("travel mode: reduced CPU allocation while on battery", "machine", env.Name, "cpus", settings.CPUs)
	}
	virtiofsRunning := false
	if settings.SharedPath != "" {
		if err := b.startVirtiofs(ctx, env.Name, settings); err != nil {
			return err
		}
		virtiofsRunning = true
	}

	args := []string{
		"-name", env.Name,
		"-m", strconv.Itoa(settings.MemoryMiB),
		"-smp", strconv.Itoa(settings.CPUs),
		"-enable-kvm",
		"-drive", fmt.Sprintf("file=%s,if=virtio,format=qcow2", b.diskPath(env.Name)),
		// Absolute pointer: the viewer sends guest coordinates, never
		// relative motion that drifts from the host cursor.
		"-device", "qemu-xhci",
		"-device", "usb-tablet",
		"-device", "virtio-serial-pci",
		"-chardev", "qemu-vdagent,id=clipboard,clipboard=on,mouse=off",
		"-device", "virtserialport,chardev=clipboard,name=com.redhat.spice.0",
		"-chardev", "socket,path=" + b.qgaPath(env.Name) + ",server=on,wait=off,id=qga0",
		"-device", "virtserialport,chardev=qga0,name=org.qemu.guest_agent.0",
		"-audiodev", "pipewire,id=audio0",
		"-device", "virtio-sound-pci,audiodev=audio0",
		"-qmp", "unix:" + b.qmpPath(env.Name) + ",server,nowait",
		"-pidfile", b.pidPath(env.Name),
		"-daemonize",
	}
	graphics := detectGraphics(ctx)
	vulkan := graphics.vulkan && !settings.VulkanDisabled
	slog.Info("graphics", "machine", env.Name, "opengl", graphics.openGL, "vulkan", vulkan, "detail", graphics.vulkanDetail)
	args = append(args, displayArgs(graphics.openGL, vulkan, settings.MemoryMiB, virtiofsRunning)...)
	if virtiofsRunning {
		args = append(args,
			"-chardev", "socket,id=virtiofs,path="+b.virtiofsPath(env.Name),
			"-device", "vhost-user-fs-pci,chardev=virtiofs,tag=omavm-share")
	}
	if env.Image != "" && !settings.DisconnectISO {
		args = append(args, "-cdrom", env.Image, "-boot", "order=cd,menu=on")
	}

	vsock := !settings.SSHDisabled && canOpenRW(vhostVsockPath)
	var cid uint32
	if vsock {
		if cid, err = b.guestCID(env.Name, false); err != nil {
			return err
		}
	}
	out, err := runQEMU(ctx, "qemu-system-x86_64", withVsock(args, vsock, cid)...)
	if err != nil && vsock && cidTaken(out) {
		// Another VM on this host holds the CID: draw a new one. The
		// guest's SSH host key is remembered per Machine, not per CID.
		if cid, err = b.guestCID(env.Name, true); err != nil {
			return err
		}
		out, err = runQEMU(ctx, "qemu-system-x86_64", withVsock(args, vsock, cid)...)
	}
	if err != nil {
		// Two concurrent Start calls can both observe "not running" above
		// before either qemu-system-x86_64 process exists (docs/TODO.md
		// P0, reproduced 2026-09-27: two `omavm start` fired in parallel
		// against the same stopped Machine). qemu-system-x86_64's own
		// -pidfile uses a real flock, so the loser here reliably fails
		// fast with "cannot create PID file" instead of actually running
		// a second instance against the same disk — but surfacing that
		// raw error to whichever caller lost the race would look like a
		// genuine startup failure. If the Machine is actually running
		// now (the other Start won), this call's goal was met: treat it
		// as the idempotent success it already is a few lines up when
		// isRunning() finds it running before even trying.
		if running, runningErr := b.isRunning(env.Name); runningErr == nil && running {
			return nil
		}
		if virtiofsRunning {
			_ = b.stopVirtiofs(env.Name)
		}
		return qemuErr("qemu-system-x86_64", err, out)
	}
	return nil
}

// virtiofsdPath is overridable in tests, like the host paths in hostcaps.go.
var virtiofsdPath = func() (string, error) {
	if path, err := exec.LookPath("virtiofsd"); err == nil {
		return path, nil
	}
	if info, err := os.Stat("/usr/lib/virtiofsd"); err == nil && info.Mode().Perm()&0o111 != 0 {
		return "/usr/lib/virtiofsd", nil
	}
	return "", fmt.Errorf("virtiofsd not found (install the virtiofsd package)")
}

// qemuBinaryPath resolves name (qemu-img or qemu-system-x86_64) via PATH,
// giving the same kind of actionable, install-hint error virtiofsdPath
// already provides instead of letting a missing binary surface as Go's
// raw "executable file not found in $PATH".
func qemuBinaryPath(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("%s not found (install it with: sudo pacman -S qemu-desktop)", name)
}

// displayArgs returns the display device and backend and, when something
// needs to share guest RAM with another process, a memfd memory backend.
// virtiofsd maps guest RAM directly; Venus (Vulkan) blob resources are
// exported to the host GPU as dma-bufs through udmabuf, which also needs
// memfd-backed RAM.
//
// The display is QEMU's D-Bus display in peer-to-peer mode: nothing is
// listening until Open hands the viewer a connection over QMP. With a
// usable host GPU, frames reach the viewer as dma-bufs (no copies through
// the CPU); without one, the guest gets a plain virtio-vga and frames are
// shared through memory mappings.
func displayArgs(openGL, vulkan bool, memoryMiB int, virtiofs bool) []string {
	vulkan = vulkan && openGL
	device, display := "virtio-vga", "dbus,p2p=yes"
	if openGL {
		device, display = "virtio-vga-gl", "dbus,p2p=yes,gl=on"
		if vulkan {
			// hostmem sizes the PCI BAR host-visible Vulkan memory is
			// mapped through: it reserves guest address space, not host RAM.
			device += ",blob=on,hostmem=4G,venus=on"
		}
	}
	args := []string{"-device", device, "-display", display}
	if vulkan || virtiofs {
		args = append(args,
			"-object", fmt.Sprintf("memory-backend-memfd,id=mem,size=%dM,share=on", memoryMiB),
			"-machine", "memory-backend=mem")
	}
	return args
}

func runQEMU(ctx context.Context, name string, args ...string) (string, error) {
	path, err := qemuBinaryPath(name)
	if err != nil {
		return "", err
	}
	return runOutput(ctx, path, args...)
}

// qemuErr wraps a failed qemu-img/qemu-system-x86_64 invocation with the
// action that failed. out is empty when the binary itself was missing
// (qemuBinaryPath already produced a complete message), so it is only
// appended when there is command output to show.
func qemuErr(action string, err error, out string) error {
	if out == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, out)
}

func (b *Backend) startVirtiofs(ctx context.Context, name string, settings core.EnvironmentSettings) error {
	// Checked here, not only when configured: the folder may have been
	// moved or deleted since, and the Machine should say so plainly.
	if info, err := os.Stat(settings.SharedPath); err != nil || !info.IsDir() {
		return core.Invalidf("shared folder %s no longer exists or is not a folder; choose another one in Settings or remove it", settings.SharedPath)
	}
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
	// virtiofsd outlives this command, so its stderr goes to a file: a pipe
	// read by this short-lived process would break (SIGPIPE) once it exits.
	logPath := filepath.Join(b.dir(name), "virtiofsd.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("shared folder: %w", err)
	}
	defer logFile.Close()
	cmd := exec.CommandContext(context.Background(), path, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start virtiofsd: %w", err)
	}
	if err := os.WriteFile(b.virtiofsPIDPath(name), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("record virtiofsd pid: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.After(2 * time.Second)
	for {
		if _, err := os.Stat(b.virtiofsPath(name)); err == nil {
			return nil
		}
		select {
		case waitErr := <-exited:
			_ = os.Remove(b.virtiofsPIDPath(name))
			return fmt.Errorf("shared folder %s could not be shared: %s", settings.SharedPath, virtiofsdFailure(logPath, waitErr))
		case <-ctx.Done():
			_ = b.stopVirtiofs(name)
			return ctx.Err()
		case <-deadline:
			_ = b.stopVirtiofs(name)
			return fmt.Errorf("shared folder %s could not be shared: virtiofsd did not start in time", settings.SharedPath)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// virtiofsdFailure is the end of virtiofsd's own output, which names the
// actual problem (permissions, sandboxing), or its exit status.
func virtiofsdFailure(logPath string, waitErr error) string {
	data, _ := os.ReadFile(logPath)
	text := strings.TrimSpace(string(data))
	if len(text) > 500 {
		text = text[len(text)-500:]
	}
	if text == "" && waitErr != nil {
		return "virtiofsd " + waitErr.Error()
	}
	return text
}

func (b *Backend) stopVirtiofs(name string) error {
	data, err := os.ReadFile(b.virtiofsPIDPath(name))
	if err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil && processHasArg(pid, b.virtiofsPath(name)) {
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
		return core.Unsupportedf("omavm-gui not found (install OmaVM's GUI to view a Machine's display)")
	}
	display, err := b.attachDisplay(env.Name)
	if err != nil {
		return err
	}
	defer display.Close()

	// Detached on purpose: the viewer is a GUI the user drives for a
	// while, not something that should die when this call's context
	// ends (same reasoning as the GUI's own openInTerminal). It inherits
	// its end of the display connection as fd 3.
	settings := env.EffectiveSettings()
	cmd := exec.CommandContext(context.Background(), viewer,
		"--display-fd", "3",
		"--title", env.Name+" — OmaVM",
		"--share-clipboard", strconv.FormatBool(!settings.ClipboardDisabled))
	cmd.ExtraFiles = []*os.File{display}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch display viewer: %w", err)
	}
	return nil
}

// attachDisplay opens a new peer-to-peer connection to the Machine's D-Bus
// display: one end of a socket pair goes to QEMU over QMP, the other is
// returned for the viewer.
func (b *Backend) attachDisplay(name string) (*os.File, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("create display connection: %w", err)
	}
	ours, theirs := os.NewFile(uintptr(fds[0]), "display"), os.NewFile(uintptr(fds[1]), "display-qemu")
	defer theirs.Close()
	if err := qmpAddDisplayClient(b.qmpPath(name), theirs); err != nil {
		ours.Close()
		if strings.Contains(err.Error(), "D-Bus display is not in use") || strings.Contains(err.Error(), "not accepted in bus mode") {
			return nil, core.Unsupportedf("this Machine was started by an older OmaVM; restart it to open its display")
		}
		return nil, fmt.Errorf("connect to the Machine's display: %w", err)
	}
	return ours, nil
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
		return "", core.Unsupportedf("machine is not running")
	}

	dst := b.previewPath(env.Name)
	if err := qmpScreendump(b.qmpPath(env.Name), dst); err != nil {
		return "", fmt.Errorf("screendump: %w", err)
	}
	return dst, nil
}

// bootDisk is the QMP device name QEMU gives the Machine's disk, the
// first -drive if=virtio.
const bootDisk = "virtio0"

// Snapshots are internal qcow2 snapshots of the disk. A stopped Machine's
// disk is changed directly with qemu-img. A running one gets a disk-only
// snapshot through QMP, as if the power had been cut at that moment: a
// full savevm (memory included) is refused by devices every Machine has
// (tested on QEMU 11.1: "State blocked by non-migratable device
// virtio-sound", and "virgl is not yet migratable" with 3D acceleration),
// so snapshots of a running Machine never worked before this.
func (b *Backend) CreateSnapshot(ctx context.Context, env core.Environment, tag string) error {
	running, err := b.isRunning(env.Name)
	if err != nil {
		return err
	}
	if !running {
		out, err := runQEMU(ctx, "qemu-img", "snapshot", "-c", tag, b.diskPath(env.Name))
		if err != nil {
			return qemuErr("qemu-img snapshot -c", err, out)
		}
		return nil
	}
	if _, err := qmpExecute(b.qmpPath(env.Name), "blockdev-snapshot-internal-sync", map[string]any{"device": bootDisk, "name": tag}); err != nil {
		return fmt.Errorf("snapshot the disk: %w", err)
	}
	return nil
}

// GoToSnapshot restores the disk to a snapshot. Only a stopped Machine:
// QEMU refuses to revert a disk-only snapshot under a running system
// ("Revert to it offline using qemu-img"), and every snapshot here is
// disk-only.
func (b *Backend) GoToSnapshot(ctx context.Context, env core.Environment, tag string) error {
	running, err := b.isRunning(env.Name)
	if err != nil {
		return err
	}
	if running {
		return core.Invalidf("shut down %s first: going to a snapshot replaces its disk, which can't change under a running system", env.Name)
	}
	out, err := runQEMU(ctx, "qemu-img", "snapshot", "-a", tag, b.diskPath(env.Name))
	if err != nil {
		return qemuErr("qemu-img snapshot -a", err, out)
	}
	return nil
}

func (b *Backend) RemoveSnapshot(ctx context.Context, env core.Environment, tag string) error {
	running, err := b.isRunning(env.Name)
	if err != nil {
		return err
	}
	if !running {
		out, err := runQEMU(ctx, "qemu-img", "snapshot", "-d", tag, b.diskPath(env.Name))
		if err != nil {
			return qemuErr("qemu-img snapshot -d", err, out)
		}
		return nil
	}
	if _, err := qmpExecute(b.qmpPath(env.Name), "blockdev-snapshot-delete-internal-sync", map[string]any{"device": bootDisk, "name": tag}); err != nil {
		return fmt.Errorf("delete the disk snapshot: %w", err)
	}
	return nil
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

	// A slow guest must never turn an ordinary shutdown into a power cut.
	if err := qmpCommand(b.qmpPath(env.Name), "system_powerdown"); err != nil {
		return fmt.Errorf("request shutdown: %w (use Force Stop only if necessary; unsaved work may be lost)", err)
	} else {
		deadline := time.Now().Add(gracefulShutdownTimeout)
		for time.Now().Before(deadline) {
			running, err := b.isRunning(env.Name)
			if err != nil {
				return err
			}
			if !running {
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
	return fmt.Errorf("shutdown is taking longer than expected; the machine was left running to protect your work. Wait or use Force Stop if necessary")
}

func (b *Backend) Restart(ctx context.Context, env core.Environment) error {
	if running, err := b.isRunning(env.Name); err != nil {
		return err
	} else if !running {
		return b.Start(ctx, env)
	}
	if err := b.Stop(ctx, env); err != nil {
		return err
	}
	return b.Start(ctx, env)
}

func (b *Backend) Pause(ctx context.Context, env core.Environment) error {
	if running, err := b.isRunning(env.Name); err != nil {
		return err
	} else if !running {
		return core.Unsupportedf("machine is not running")
	}
	return qmpCommand(b.qmpPath(env.Name), "stop")
}

func (b *Backend) Resume(ctx context.Context, env core.Environment) error {
	if running, err := b.isRunning(env.Name); err != nil {
		return err
	} else if !running {
		return core.Unsupportedf("machine is not running")
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
	if err := qmpCommand(b.qmpPath(env.Name), "quit"); err != nil {
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
	}
	_ = b.stopVirtiofs(env.Name)
	// QEMU takes a moment to exit after quit. Returning before that let a
	// Delete right after Force Stop find it still running and try a
	// normal shutdown through the QMP socket QEMU had already closed.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if running, err := b.isRunning(env.Name); err != nil || !running {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return fmt.Errorf("the machine is still shutting down; try again in a moment")
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
		return core.Status{State: core.StateRunning, Detail: "display available"}
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
	return processHasArg(pid, b.pidPath(name)), nil
}

// processHasArg reports whether pid is running with arg on its command
// line, which is how a pidfile is tied to the process OmaVM started (QEMU
// gets -pidfile <path>, virtiofsd --socket-path <path>). A pid alone is not
// enough: after a crash or a reboot the pidfile stays behind, and its
// number can belong to any other process by then. Trusting it showed the
// Machine as running, made Start a silent no-op, and sent SIGTERM to that
// stranger on Force Stop.
func processHasArg(pid int, arg string) bool {
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	for _, a := range strings.Split(string(cmdline), "\x00") {
		if a == arg {
			return true
		}
	}
	return false
}

func runOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
