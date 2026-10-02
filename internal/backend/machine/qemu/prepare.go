package qemu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// Preparing a guest goes through qemu-guest-agent, which runs commands as
// root inside the guest (guest-exec): OmaVM puts nothing of its own in the
// guest. The one thing it can't do is install that agent itself, so a
// guest without it gets the command to run.

const agentInstallHint = "Install and start qemu-guest-agent in the guest first (Fedora: sudo dnf install qemu-guest-agent; Ubuntu and Debian: sudo apt install qemu-guest-agent; Arch: sudo pacman -S qemu-guest-agent), then prepare it again"

// Where the shared folder is mounted in the guest, and how fstab names it.
const (
	guestShareMount = "/mnt/omavm-share"
	shareFstabLine  = "omavm-share " + guestShareMount + " virtiofs defaults,nofail 0 0"
)

// errExecBlocked is a guest agent that won't run commands: RHEL and its
// rebuilds ship qemu-ga with guest-exec off.
var errExecBlocked = errors.New("the guest agent doesn't allow running commands")

// Timeouts for one command in the guest: installing a package downloads
// it; checks read a file or two.
const (
	installTimeout = 10 * time.Minute
	checkTimeout   = 30 * time.Second
)

// guestExec runs a shell script as root in the guest and returns its exit
// code and output (stdout, then stderr).
func guestExec(ctx context.Context, socketPath, script string, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, dec, err := qgaDial(ctx, socketPath)
	if err != nil {
		return 0, "", err
	}
	defer conn.Close()
	raw, err := qgaCall(conn, dec, "guest-exec", map[string]any{
		"path": "/bin/sh", "arg": []string{"-c", script}, "capture-output": true,
	}, 5*time.Second)
	if err != nil {
		if msg := err.Error(); strings.Contains(msg, "disabled") || strings.Contains(msg, "CommandNotFound") {
			return 0, "", errExecBlocked
		}
		return 0, "", err
	}
	var started struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(raw, &started); err != nil {
		return 0, "", fmt.Errorf("guest-exec: %w", err)
	}
	for {
		raw, err := qgaCall(conn, dec, "guest-exec-status", map[string]any{"pid": started.PID}, 5*time.Second)
		if err != nil {
			return 0, "", err
		}
		var status struct {
			Exited   bool   `json:"exited"`
			ExitCode int    `json:"exitcode"`
			Signal   int    `json:"signal"`
			OutData  string `json:"out-data"`
			ErrData  string `json:"err-data"`
		}
		if err := json.Unmarshal(raw, &status); err != nil {
			return 0, "", fmt.Errorf("guest-exec-status: %w", err)
		}
		if status.Exited {
			out, _ := base64.StdEncoding.DecodeString(status.OutData)
			errOut, _ := base64.StdEncoding.DecodeString(status.ErrData)
			code := status.ExitCode
			if status.Signal != 0 {
				code = 128 + status.Signal
			}
			return code, strings.TrimSpace(string(out) + "\n" + string(errOut)), nil
		}
		select {
		case <-ctx.Done():
			return 0, "", fmt.Errorf("the guest didn't finish in %s", timeout)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// lastLines keeps the end of a command's output, which names the problem.
func lastLines(out string, n int) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

// installClipboardAgent installs spice-vdagent with whichever package
// manager the guest has, and starts its system service.
const installClipboardAgent = `
if command -v spice-vdagent >/dev/null 2>&1; then exit 0; fi
if command -v dnf >/dev/null 2>&1; then dnf install -y spice-vdagent
elif command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -q && apt-get install -y -q spice-vdagent
elif command -v pacman >/dev/null 2>&1; then pacman -S --noconfirm --needed spice-vdagent
elif command -v zypper >/dev/null 2>&1; then zypper --non-interactive install spice-vdagent
else echo "no supported package manager (dnf, apt-get, pacman or zypper)" >&2; exit 3
fi || exit $?
systemctl enable --now spice-vdagentd.socket >/dev/null 2>&1 || systemctl enable --now spice-vdagentd >/dev/null 2>&1 || true
`

// manualMount is mountSharedFolder as one line a person can paste.
const manualMount = "sudo mkdir -p " + guestShareMount + " && echo '" + shareFstabLine + "' | sudo tee -a /etc/fstab && sudo mount " + guestShareMount

// mountSharedFolder mounts the shared folder now and at every boot
// (nofail: a boot without it, after it is unshared, still succeeds).
const mountSharedFolder = `
set -e
mkdir -p ` + guestShareMount + `
grep -qs '^omavm-share[[:space:]]' /etc/fstab || echo '` + shareFstabLine + `' >> /etc/fstab
systemctl daemon-reload >/dev/null 2>&1 || true
mountpoint -q ` + guestShareMount + ` || mount ` + guestShareMount + `
`

// PrepareGuest implements core.GuestPreparer for Linux guests.
func (b *Backend) PrepareGuest(ctx context.Context, env core.Environment) ([]core.PreparationStep, error) {
	key := b.key(env)
	if running, err := b.isRunning(key); err != nil {
		return nil, err
	} else if !running {
		return nil, core.Invalidf("start %s first: its guest is prepared from inside, while it runs", env.Name)
	}
	if err := qgaPing(ctx, b.qgaPath(key)); err != nil {
		return nil, core.Unsupportedf("%s has no guest agent answering. %s", env.Name, agentInstallHint)
	}
	blocked := func(err error) error {
		return core.Unsupportedf("%s: %v. On RHEL-based guests, allow guest-exec in /etc/sysconfig/qemu-ga, then restart qemu-guest-agent", env.Name, err)
	}
	guest, err := checkLinuxGuest(ctx, b.qgaPath(key), env.Name)
	if errors.Is(err, errExecBlocked) {
		return nil, blocked(err)
	} else if err != nil {
		return nil, err
	}
	guest.confined = agentConfined(ctx, b.qgaPath(key))

	settings := env.EffectiveSettings()
	channels, _ := guestChannels(b.qmpPath(key))
	var steps []core.PreparationStep
	add := func(step core.PreparationStep, err error) error {
		if errors.Is(err, errExecBlocked) {
			return blocked(err)
		}
		if err != nil {
			step.Result, step.Detail = core.StepFailed, err.Error()
		}
		steps = append(steps, step)
		return nil
	}

	for _, run := range []func() (core.PreparationStep, error){
		func() (core.PreparationStep, error) { return b.prepareClipboard(ctx, env, guest, settings, channels) },
		func() (core.PreparationStep, error) { return b.prepareSharedFolder(ctx, env, guest, settings) },
		func() (core.PreparationStep, error) { return checkGuestAudio(ctx, b.qgaPath(key)) },
		func() (core.PreparationStep, error) { return checkGuestDisplay(ctx, b.qgaPath(key)) },
	} {
		if err := add(run()); err != nil {
			return nil, err
		}
	}
	return steps, nil
}

// linuxGuest is what preparing needs to know about the guest system.
type linuxGuest struct {
	id, prettyName string
	// confined: the agent runs under an SELinux domain (Fedora, RHEL)
	// that lets it read the system but not install software or write
	// /etc, so those steps become commands for the person to run.
	confined bool
}

// checkLinuxGuest refuses guests the scripts aren't for. The agent's
// osinfo names Windows by its id; any other system says what it is through
// uname, which also shows whether the agent runs commands at all.
func checkLinuxGuest(ctx context.Context, socketPath, name string) (linuxGuest, error) {
	conn, dec, err := qgaDial(ctx, socketPath)
	if err != nil {
		return linuxGuest{}, err
	}
	raw, err := qgaQuery(conn, dec, "guest-get-osinfo", 5*time.Second)
	conn.Close()
	if err != nil {
		return linuxGuest{}, err
	}
	var info struct {
		ID         string `json:"id"`
		PrettyName string `json:"pretty-name"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return linuxGuest{}, fmt.Errorf("guest-get-osinfo: %w", err)
	}
	guest := linuxGuest{id: info.ID, prettyName: info.PrettyName}
	if info.ID == "mswindows" {
		return guest, core.Unsupportedf("%s runs %s; preparing a guest only works for Linux so far", name, info.PrettyName)
	}
	code, out, err := guestExec(ctx, socketPath, "uname -s", checkTimeout)
	if err != nil {
		return guest, err
	}
	if code != 0 || strings.TrimSpace(out) != "Linux" {
		system := info.PrettyName
		if system == "" {
			system = strings.TrimSpace(out)
		}
		return guest, core.Unsupportedf("%s runs %s; preparing a guest only works for Linux so far", name, system)
	}
	return guest, nil
}

// agentConfined reads the SELinux domain commands from the agent run in.
// Fedora's policy confines qemu-ga to virt_qemu_ga_t (verified on Fedora
// 44: as root it can't run dnf or create a directory under /mnt).
func agentConfined(ctx context.Context, socketPath string) bool {
	code, out, err := guestExec(ctx, socketPath, "cat /proc/self/attr/current 2>/dev/null", checkTimeout)
	return err == nil && code == 0 && strings.Contains(out, "qemu_ga_t")
}

// installCommand is how a person installs a package in this guest.
func (g linuxGuest) installCommand(pkg string) string {
	switch g.id {
	case "fedora", "rhel", "centos", "rocky", "almalinux", "ol":
		return "sudo dnf install " + pkg
	case "ubuntu", "debian", "linuxmint", "pop", "elementary", "zorin":
		return "sudo apt install " + pkg
	case "arch", "manjaro", "endeavouros", "cachyos":
		return "sudo pacman -S " + pkg
	}
	if strings.HasPrefix(g.id, "opensuse") || g.id == "sles" {
		return "sudo zypper install " + pkg
	}
	return "install " + pkg + " with its package manager"
}

func (g linuxGuest) confinedBecause() string {
	system := g.prettyName
	if system == "" {
		system = "this guest"
	}
	return "SELinux in " + system + " keeps the guest agent from changing the system"
}

func (b *Backend) prepareClipboard(ctx context.Context, env core.Environment, guest linuxGuest, settings core.EnvironmentSettings, channels map[string]bool) (core.PreparationStep, error) {
	step := core.PreparationStep{ID: "clipboard", Label: "Clipboard"}
	switch {
	case settings.ClipboardDisabled:
		step.Result, step.Detail = core.StepSkipped, "Turned off in Settings"
		return step, nil
	case channels["clipboard"]:
		step.Result, step.Detail = core.StepReady, "Text already copies both ways while its window is active"
		return step, nil
	case guest.confined:
		step.Result, step.Detail = core.StepManual, guest.confinedBecause()+". In the guest, run: "+guest.installCommand("spice-vdagent")+", then sign out of its desktop and back in"
		return step, nil
	}
	core.ReportProgress(ctx, "Installing the clipboard agent in %s", env.Name)
	code, out, err := guestExec(ctx, b.qgaPath(b.key(env)), installClipboardAgent, installTimeout)
	if err != nil {
		return step, err
	}
	if code != 0 {
		return step, fmt.Errorf("installing spice-vdagent failed (exit %d): %s", code, lastLines(out, 3))
	}
	if open, _ := guestChannels(b.qmpPath(b.key(env))); open["clipboard"] {
		step.Result, step.Detail = core.StepDone, "Text copies both ways while its window is active"
		return step, nil
	}
	// spice-vdagent's session part starts with the guest's desktop.
	step.Result, step.Detail = core.StepManual, "spice-vdagent is installed; sign out of the guest's desktop and back in for the clipboard to connect"
	return step, nil
}

func (b *Backend) prepareSharedFolder(ctx context.Context, env core.Environment, guest linuxGuest, settings core.EnvironmentSettings) (core.PreparationStep, error) {
	step := core.PreparationStep{ID: "shared_folder", Label: "Shared folder"}
	key := b.key(env)
	if settings.SharedPath == "" {
		step.Result, step.Detail = core.StepSkipped, "Choose a folder in Settings to share it"
		return step, nil
	}
	if a, ok := b.appliedConfig(key); !ok || !a.virtiofs {
		step.Result, step.Detail = core.StepManual, "Restart the Machine to share "+settings.SharedPath+", then prepare it again"
		return step, nil
	}
	if mount, err := virtiofsMountpoint(ctx, b.qgaPath(key)); err == nil && mount != "" {
		step.Result, step.Detail = core.StepReady, fmt.Sprintf("%s is already at %s in the guest", settings.SharedPath, mount)
		return step, nil
	}
	if guest.confined {
		step.Result, step.Detail = core.StepManual, guest.confinedBecause()+". In the guest, run: "+manualMount
		return step, nil
	}
	core.ReportProgress(ctx, "Mounting the shared folder in %s", env.Name)
	code, out, err := guestExec(ctx, b.qgaPath(key), mountSharedFolder, checkTimeout)
	if err != nil {
		return step, err
	}
	if code != 0 {
		return step, fmt.Errorf("mounting the shared folder failed (exit %d): %s", code, lastLines(out, 3))
	}
	// Confirmed the way the integration report checks it, not by the
	// script's word.
	mount, err := virtiofsMountpoint(ctx, b.qgaPath(key))
	if err != nil || mount == "" {
		return step, fmt.Errorf("the shared folder was mounted but the guest doesn't list it (%v)", err)
	}
	step.Result, step.Detail = core.StepDone, fmt.Sprintf("%s is at %s in the guest, mounted again at every boot", settings.SharedPath, mount)
	return step, nil
}

// checkGuestAudio sees whether the guest's kernel drives the virtio sound
// card; there is nothing to install for it in userspace.
func checkGuestAudio(ctx context.Context, socketPath string) (core.PreparationStep, error) {
	step := core.PreparationStep{ID: "audio", Label: "Sound"}
	code, _, err := guestExec(ctx, socketPath, "test -d /sys/bus/virtio/drivers/virtio_snd", checkTimeout)
	if err != nil {
		return step, err
	}
	if code == 0 {
		step.Result, step.Detail = core.StepReady, "The guest's sound card plays through this computer"
	} else {
		step.Result, step.Detail = core.StepManual, "The guest's kernel has no driver for its sound card (virtio_snd, in Linux 5.13 and newer); some systems ship it in a separate kernel modules package"
	}
	return step, nil
}

// checkGuestDisplay sees whether the guest uses the virtio GPU driver,
// without which its resolution can't follow the window.
func checkGuestDisplay(ctx context.Context, socketPath string) (core.PreparationStep, error) {
	step := core.PreparationStep{ID: "display", Label: "Resolution"}
	code, _, err := guestExec(ctx, socketPath, "test -d /sys/bus/virtio/drivers/virtio_gpu", checkTimeout)
	if err != nil {
		return step, err
	}
	if code == 0 {
		step.Result, step.Detail = core.StepReady, "The guest's resolution follows the window when its desktop supports it"
	} else {
		step.Result, step.Detail = core.StepManual, "The guest isn't using the virtio GPU driver (virtio_gpu), so its resolution stays fixed"
	}
	return step, nil
}
