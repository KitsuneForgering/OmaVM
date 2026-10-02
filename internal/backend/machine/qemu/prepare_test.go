package qemu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// fakeGuest is a running Linux guest as its agent and QEMU's QMP show it.
// run answers each script guest-exec is given; it may change the guest
// (mount the share, open the clipboard channel).
type fakeGuest struct {
	mu            sync.Mutex
	kernel        string
	execBlocked   bool
	selinux       bool // the agent is confined as on Fedora
	mounted       bool
	clipboardOpen bool
	scripts       []string
	run           func(g *fakeGuest, script string) (int, string)
}

func (g *fakeGuest) ran(part string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, s := range g.scripts {
		if strings.Contains(s, part) {
			return true
		}
	}
	return false
}

func (g *fakeGuest) serveAgent(t *testing.T, socket string) {
	t.Helper()
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	type result struct {
		code int
		out  string
	}
	var results sync.Map
	nextPID := 100
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				dec, enc := json.NewDecoder(conn), json.NewEncoder(conn)
				for {
					var req struct {
						Execute   string         `json:"execute"`
						Arguments map[string]any `json:"arguments"`
					}
					if dec.Decode(&req) != nil {
						return
					}
					g.mu.Lock()
					var reply any
					switch req.Execute {
					case "guest-sync":
						reply = map[string]any{"return": req.Arguments["id"]}
					case "guest-ping":
						reply = map[string]any{"return": map[string]any{}}
					case "guest-get-osinfo":
						// The shape a real agent sends (QEMU 10.2 on Fedora
						// 44): no kernel name; Windows is told by its id.
						if g.kernel == "Windows" {
							reply = map[string]any{"return": map[string]any{"id": "mswindows", "pretty-name": "Windows 11 Pro", "kernel-release": "26100"}}
						} else {
							reply = map[string]any{"return": map[string]any{"id": "fedora", "pretty-name": "Fedora Linux 44 (Cloud Edition)", "kernel-release": "6.19.10-300.fc44.x86_64"}}
						}
					case "guest-get-fsinfo":
						// Like a real agent: only disk-backed filesystems,
						// never virtiofs.
						reply = map[string]any{"return": []map[string]any{{"mountpoint": "/", "type": "btrfs"}}}
					case "guest-exec":
						if g.execBlocked {
							reply = map[string]any{"error": map[string]any{"class": "GenericError", "desc": "Command guest-exec has been disabled"}}
							break
						}
						args, _ := req.Arguments["arg"].([]any)
						script := ""
						if req.Arguments["path"] == "/bin/sh" && len(args) == 2 && args[0] == "-c" {
							script, _ = args[1].(string)
						}
						g.scripts = append(g.scripts, script)
						g.mu.Unlock()
						code, out := g.run(g, script)
						g.mu.Lock()
						nextPID++
						results.Store(nextPID, result{code, out})
						reply = map[string]any{"return": map[string]any{"pid": nextPID}}
					case "guest-exec-status":
						pid := int(req.Arguments["pid"].(float64))
						r, _ := results.Load(pid)
						res := r.(result)
						reply = map[string]any{"return": map[string]any{
							"exited": true, "exitcode": res.code,
							"out-data": base64.StdEncoding.EncodeToString([]byte(res.out)),
						}}
					default:
						reply = map[string]any{"error": map[string]any{"class": "CommandNotFound", "desc": req.Execute}}
					}
					g.mu.Unlock()
					_ = enc.Encode(reply)
				}
			}(conn)
		}
	}()
}

// serveQMP answers query-chardev with the clipboard channel's state.
func (g *fakeGuest) serveQMP(t *testing.T, socket string) {
	t.Helper()
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			enc, dec := json.NewEncoder(conn), json.NewDecoder(conn)
			_ = enc.Encode(map[string]any{"QMP": map[string]any{}})
			for {
				var req struct {
					Execute string `json:"execute"`
				}
				if dec.Decode(&req) != nil {
					break
				}
				g.mu.Lock()
				open := g.clipboardOpen
				g.mu.Unlock()
				if req.Execute == "query-chardev" {
					_ = enc.Encode(map[string]any{"return": []map[string]any{{"label": "clipboard", "frontend-open": open}, {"label": "qga0", "frontend-open": true}}})
				} else {
					_ = enc.Encode(map[string]any{"return": map[string]any{}})
				}
			}
			conn.Close()
		}
	}()
}

// guestWorks answers every script as a cooperative Fedora guest would:
// installs succeed (the channel opens only after a new login), the mount
// script mounts, the sound card is there, the GPU driver isn't.
func guestWorks(g *fakeGuest, script string) (int, string) {
	switch {
	case script == "uname -s":
		g.mu.Lock()
		defer g.mu.Unlock()
		return 0, g.kernel
	case strings.Contains(script, "spice-vdagent"):
		return 0, "Installed: spice-vdagent"
	case strings.Contains(script, "fstab"):
		g.mu.Lock()
		g.mounted = true
		g.mu.Unlock()
		return 0, ""
	case strings.Contains(script, "/proc/mounts"):
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.mounted {
			return 0, guestShareMount + "\n"
		}
		return 0, ""
	case strings.Contains(script, "virtio_snd"):
		return 0, ""
	case strings.Contains(script, "attr/current"):
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.selinux {
			return 0, "system_u:system_r:virt_qemu_ga_t:s0"
		}
		return 0, "unconfined"
	default:
		return 1, ""
	}
}

// runningMachine starts a fake QEMU for a Machine (with the shared folder
// in its session when shared is set) and a fake guest behind it.
func runningMachine(t *testing.T, shared bool, guest *fakeGuest) (*Backend, core.Environment) {
	t.Helper()
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "fed", Kind: core.Machine}
	var extra []string
	if shared {
		env.Settings.SharedPath = "/home/ana/Projetos"
		extra = []string{"-device", "vhost-user-fs-pci,chardev=virtiofs,tag=omavm-share"}
	}
	startFakeQEMU(t, b, b.key(env), extra...)
	if guest.kernel == "" {
		guest.kernel = "Linux"
	}
	guest.serveAgent(t, b.qgaPath(b.key(env)))
	guest.serveQMP(t, b.qmpPath(b.key(env)))
	return b, env
}

func resultsOf(steps []core.PreparationStep) map[string]core.PreparationStep {
	m := map[string]core.PreparationStep{}
	for _, s := range steps {
		m[s.ID] = s
	}
	return m
}

func TestPrepareGuestSetsUpWhatsMissing(t *testing.T) {
	guest := &fakeGuest{run: guestWorks}
	b, env := runningMachine(t, true, guest)
	steps, err := b.PrepareGuest(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	got := resultsOf(steps)
	want := map[string]string{"clipboard": core.StepManual, "shared_folder": core.StepDone, "audio": core.StepReady, "display": core.StepManual}
	for id, result := range want {
		if got[id].Result != result {
			t.Errorf("%s = %s (%s), want %s", id, got[id].Result, got[id].Detail, result)
		}
	}
	if !strings.Contains(got["clipboard"].Detail, "sign out") {
		t.Errorf("clipboard should say to sign in again: %q", got["clipboard"].Detail)
	}
	if !strings.Contains(got["shared_folder"].Detail, guestShareMount) {
		t.Errorf("shared folder should say where it is: %q", got["shared_folder"].Detail)
	}
	if !guest.ran(shareFstabLine) || !guest.ran("nofail") {
		t.Error("the share must be added to fstab with nofail, so it mounts at boot without blocking it")
	}
}

// After a new login spice-vdagent opens the channel: preparing says it
// works, instead of asking for the login.
func TestPrepareGuestConfirmsTheClipboardWhenItConnects(t *testing.T) {
	guest := &fakeGuest{run: func(g *fakeGuest, script string) (int, string) {
		if strings.Contains(script, "spice-vdagent") {
			g.mu.Lock()
			g.clipboardOpen = true
			g.mu.Unlock()
		}
		return guestWorks(g, script)
	}}
	b, env := runningMachine(t, false, guest)
	steps, err := b.PrepareGuest(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultsOf(steps)["clipboard"]; got.Result != core.StepDone {
		t.Errorf("clipboard = %+v", got)
	}
}

// What already works is left alone: nothing is installed or mounted.
func TestPrepareGuestLeavesWorkingIntegrationsAlone(t *testing.T) {
	guest := &fakeGuest{run: guestWorks, mounted: true, clipboardOpen: true}
	b, env := runningMachine(t, true, guest)
	steps, err := b.PrepareGuest(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	got := resultsOf(steps)
	if got["clipboard"].Result != core.StepReady || got["shared_folder"].Result != core.StepReady {
		t.Errorf("got %+v", got)
	}
	if guest.ran("spice-vdagent") || guest.ran("fstab") {
		t.Errorf("changed a guest that already worked: %q", guest.scripts)
	}
}

func TestPrepareGuestRespectsSettings(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		guest := &fakeGuest{run: guestWorks}
		b, env := runningMachine(t, false, guest)
		env.Settings.ClipboardDisabled = true
		steps, err := b.PrepareGuest(context.Background(), env)
		if err != nil {
			t.Fatal(err)
		}
		got := resultsOf(steps)
		if got["clipboard"].Result != core.StepSkipped || got["shared_folder"].Result != core.StepSkipped {
			t.Errorf("got %+v", got)
		}
		if guest.ran("spice-vdagent") || guest.ran("fstab") {
			t.Errorf("installed what is turned off: %q", guest.scripts)
		}
	})
	t.Run("restart", func(t *testing.T) {
		// The folder was chosen after this session started.
		guest := &fakeGuest{run: guestWorks}
		b, env := runningMachine(t, false, guest)
		env.Settings.SharedPath = "/home/ana/Projetos"
		steps, err := b.PrepareGuest(context.Background(), env)
		if err != nil {
			t.Fatal(err)
		}
		if got := resultsOf(steps)["shared_folder"]; got.Result != core.StepManual || !strings.Contains(got.Detail, "Restart") {
			t.Errorf("got %+v", got)
		}
		if guest.ran("fstab") {
			t.Error("mounted a folder the session doesn't share")
		}
	})
}

// A failed step says why, in the guest's own words, and the others still
// run.
func TestPrepareGuestReportsAFailedStepAndGoesOn(t *testing.T) {
	guest := &fakeGuest{run: func(g *fakeGuest, script string) (int, string) {
		if strings.Contains(script, "spice-vdagent") {
			return 1, "Updating metadata\nError: Failed to download metadata for repo 'fedora'"
		}
		return guestWorks(g, script)
	}}
	b, env := runningMachine(t, true, guest)
	steps, err := b.PrepareGuest(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	got := resultsOf(steps)
	if c := got["clipboard"]; c.Result != core.StepFailed || !strings.Contains(c.Detail, "Failed to download metadata") {
		t.Errorf("clipboard = %+v", c)
	}
	if got["shared_folder"].Result != core.StepDone {
		t.Errorf("a failed install stopped the other steps: %+v", got)
	}
}

// The mount is confirmed by asking the guest, never by the script's exit
// code alone.
func TestPrepareGuestConfirmsTheMount(t *testing.T) {
	guest := &fakeGuest{run: func(g *fakeGuest, script string) (int, string) {
		if strings.Contains(script, "fstab") {
			return 0, "" // claims success, mounts nothing
		}
		return guestWorks(g, script)
	}}
	b, env := runningMachine(t, true, guest)
	steps, err := b.PrepareGuest(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultsOf(steps)["shared_folder"]; got.Result != core.StepFailed {
		t.Errorf("an unconfirmed mount was reported as %+v", got)
	}
}

func TestPrepareGuestNeedsARunningLinuxGuestWithAnAgent(t *testing.T) {
	t.Run("stopped", func(t *testing.T) {
		b := &Backend{stateDir: t.TempDir()}
		_, err := b.PrepareGuest(context.Background(), core.Environment{Name: "fed", Kind: core.Machine})
		if !errors.Is(err, core.ErrInvalidInput) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("no agent", func(t *testing.T) {
		b := &Backend{stateDir: t.TempDir()}
		env := core.Environment{Name: "fed", Kind: core.Machine}
		startFakeQEMU(t, b, b.key(env))
		_, err := b.PrepareGuest(context.Background(), env)
		if !errors.Is(err, core.ErrUnsupported) || !strings.Contains(err.Error(), "qemu-guest-agent") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("windows", func(t *testing.T) {
		guest := &fakeGuest{kernel: "Windows", run: guestWorks}
		b, env := runningMachine(t, false, guest)
		_, err := b.PrepareGuest(context.Background(), env)
		if !errors.Is(err, core.ErrUnsupported) || !strings.Contains(err.Error(), "Linux") {
			t.Errorf("got %v", err)
		}
		if len(guest.scripts) != 0 {
			t.Errorf("ran Linux scripts in a Windows guest: %q", guest.scripts)
		}
	})
	t.Run("exec blocked", func(t *testing.T) {
		guest := &fakeGuest{execBlocked: true, run: guestWorks}
		b, env := runningMachine(t, false, guest)
		_, err := b.PrepareGuest(context.Background(), env)
		if !errors.Is(err, core.ErrUnsupported) || !strings.Contains(err.Error(), "/etc/sysconfig/qemu-ga") {
			t.Errorf("got %v", err)
		}
	})
}

// On Fedora the agent is confined by SELinux: it can't install or mount,
// so preparing says exactly what to run instead of failing on "Permission
// denied", and still checks what it can read.
func TestPrepareGuestUnderSELinuxGivesTheCommands(t *testing.T) {
	guest := &fakeGuest{run: guestWorks, selinux: true}
	b, env := runningMachine(t, true, guest)
	steps, err := b.PrepareGuest(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	got := resultsOf(steps)
	if c := got["clipboard"]; c.Result != core.StepManual || !strings.Contains(c.Detail, "sudo dnf install spice-vdagent") || !strings.Contains(c.Detail, "SELinux") {
		t.Errorf("clipboard = %+v", c)
	}
	if s := got["shared_folder"]; s.Result != core.StepManual || !strings.Contains(s.Detail, manualMount) {
		t.Errorf("shared folder = %+v", s)
	}
	if got["audio"].Result != core.StepReady {
		t.Errorf("checks must still run: %+v", got["audio"])
	}
	if guest.ran("dnf install") || guest.ran("fstab") {
		t.Errorf("tried to change a confined guest: %q", guest.scripts)
	}
}

func TestInstallCommandPerSystem(t *testing.T) {
	for id, want := range map[string]string{
		"fedora": "sudo dnf install spice-vdagent", "ubuntu": "sudo apt install spice-vdagent",
		"arch": "sudo pacman -S spice-vdagent", "opensuse-tumbleweed": "sudo zypper install spice-vdagent",
		"gentoo": "install spice-vdagent with its package manager",
	} {
		if got := (linuxGuest{id: id}).installCommand("spice-vdagent"); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
}
