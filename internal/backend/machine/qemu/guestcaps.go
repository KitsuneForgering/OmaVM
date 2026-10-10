package qemu

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// prepareHint names the action that does a guest step for the person.
const prepareHint = "Prepare the Guest (omavm prepare) does it through the guest agent"

// mountHint is the same mount Prepare the Guest sets up, kept across boots.
const mountHint = "In a Linux guest: " + manualMount

// guestChecks is what could be checked on a running Machine's guest side;
// nil pointers mean the check couldn't run.
type guestChecks struct {
	agent            bool    // qemu-guest-agent answers
	clipboardOpen    *bool   // spice-vdagent opened the clipboard channel
	virtiofsMount    *string // where virtiofs is mounted ("" = nowhere)
	sessionHasShared bool    // the session was started with the folder
}

func clipboardReadyHint(settings core.EnvironmentSettings) string {
	switch settings.ClipboardDirection {
	case core.ClipboardToHost:
		return "Guest clipboard agent connected for copying from the guest to this computer; actual copy/paste still needs a test while its window is active"
	case core.ClipboardToGuest:
		return "Guest clipboard agent connected for copying from this computer to the guest; actual copy/paste still needs a test while its window is active"
	default:
		return "Guest clipboard agent connected for copying both ways; actual copy/paste still needs a test while its window is active"
	}
}

// guestCapabilities turns settings and checks into states with a next
// step. A stopped Machine (checks == nil) is never reported ready.
func guestCapabilities(env core.Environment, checks *guestChecks) []core.GuestCapability {
	settings := machineSettings(env)
	clipboard := core.GuestCapability{ID: "clipboard", Label: "Clipboard"}
	switch {
	case settings.ClipboardDisabled:
		clipboard.State, clipboard.Hint = core.GuestOff, "Turned off in Settings"
	case checks == nil || checks.clipboardOpen == nil:
		clipboard.State, clipboard.Hint = core.GuestNotVerified, "Checked while the Machine is running"
	case *checks.clipboardOpen:
		clipboard.State, clipboard.Hint = core.GuestReady, clipboardReadyHint(settings)
	default:
		clipboard.State, clipboard.Hint = core.GuestNeedsComponent, "Install spice-vdagent in the guest and sign in to its desktop. "+prepareHint
	}

	shared := core.GuestCapability{ID: "shared_folder", Label: "Shared folder"}
	switch {
	case settings.SharedPath == "":
		shared.State, shared.Hint = core.GuestOff, "Turned off in Settings"
	case checks == nil:
		shared.State, shared.Hint = core.GuestNotVerified, "Checked while the Machine is running. "+mountHint
	case !checks.sessionHasShared:
		shared.State, shared.Hint = core.GuestNeedsRestart, "Restart the Machine to share "+settings.SharedPath
	case !checks.agent || checks.virtiofsMount == nil:
		shared.State, shared.Hint = core.GuestNotVerified, "Install qemu-guest-agent in the guest to check it. "+mountHint
	case *checks.virtiofsMount != "":
		shared.State, shared.Hint = core.GuestReady, fmt.Sprintf("%s is mounted at %s in the guest; read/write still needs a file test", settings.SharedPath, *checks.virtiofsMount)
	default:
		shared.State, shared.Hint = core.GuestNeedsComponent, prepareHint+". "+mountHint
	}
	return []core.GuestCapability{clipboard, shared}
}

// guestChannels reports which virtio-serial ports the guest has opened,
// by chardev label: "clipboard" when spice-vdagent runs, "qga0" when
// qemu-guest-agent does. QEMU shows it as the chardev's frontend being
// open, so this needs nothing from the guest itself.
func guestChannels(qmpPath string) (map[string]bool, error) {
	raw, err := qmpExecute(qmpPath, "query-chardev", nil)
	if err != nil {
		return nil, err
	}
	var chardevs []struct {
		Label        string `json:"label"`
		FrontendOpen bool   `json:"frontend-open"`
	}
	if err := json.Unmarshal(raw, &chardevs); err != nil {
		return nil, err
	}
	open := map[string]bool{}
	for _, c := range chardevs {
		open[c.Label] = c.FrontendOpen
	}
	return open, nil
}

func (b *Backend) guestChecks(ctx context.Context, env core.Environment, agent bool, channels map[string]bool) *guestChecks {
	key := b.key(env)
	c := &guestChecks{agent: agent}
	if open, ok := channels["clipboard"]; ok {
		c.clipboardOpen = &open
	}
	if a, ok := b.appliedConfig(key); ok {
		c.sessionHasShared = a.virtiofs
	}
	if agent && c.sessionHasShared {
		check := func() (string, error) { return virtiofsMountpoint(ctx, b.qgaPath(key)) }
		if mount, err := b.cachedMount(key, check); err == nil {
			c.virtiofsMount = &mount
		}
	}
	return c
}

// mountCacheTTL is how long the integration report trusts where the guest
// mounted the shared folder. Finding out runs a command in the guest, and
// the list is polled every 3 s while it only changes when someone mounts
// or unmounts the folder.
const mountCacheTTL = time.Minute

func (b *Backend) mountCachePath(name string) string {
	return filepath.Join(b.dir(name), "virtiofs-mount.json")
}

type mountCache struct {
	PID     int       `json:"pid"`
	Mount   string    `json:"mount"`
	Checked time.Time `json:"checked"`
}

// cachedMount answers from the last check of this QEMU session (the pid
// ties it to one) for mountCacheTTL; failures are never kept.
func (b *Backend) cachedMount(name string, check func() (string, error)) (string, error) {
	pid, err := b.readPID(name)
	if err != nil {
		return check()
	}
	var cached mountCache
	if data, err := os.ReadFile(b.mountCachePath(name)); err == nil &&
		json.Unmarshal(data, &cached) == nil && cached.PID == pid {
		if age := time.Since(cached.Checked); age >= 0 && age < mountCacheTTL {
			return cached.Mount, nil
		}
	}
	mount, err := check()
	if err != nil {
		return "", err
	}
	if data, err := json.Marshal(mountCache{PID: pid, Mount: mount, Checked: time.Now()}); err == nil {
		_ = os.WriteFile(b.mountCachePath(name), data, 0o600)
	}
	return mount, nil
}

// forgetMount drops the cached answer after the mount changed on purpose.
func (b *Backend) forgetMount(name string) { _ = os.Remove(b.mountCachePath(name)) }
