package qemu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

func TestStatusFromQMP(t *testing.T) {
	tests := map[string]core.State{
		"running":        core.StateRunning,
		"paused":         core.StatePaused,
		"suspended":      core.StatePaused,
		"inmigrate":      core.StateStarting,
		"shutdown":       core.StateStopping,
		"guest-panicked": core.StateError,
		"colo":           core.StateUnknown,
	}
	for input, want := range tests {
		if got := statusFromQMP(input).State; got != want {
			t.Errorf("statusFromQMP(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCreateRejectsNameTooLongForSocketPath(t *testing.T) {
	b := &Backend{stateDir: "/home/user/.local/state/omavm/machines"}
	// "<stateDir>/<name>/virtiofs.sock": one more "/" than virtiofsPath("").
	fits := strings.Repeat("a", maxSocketPath-len(b.virtiofsPath(""))-1)
	if err := b.checkNameLength(fits); err != nil {
		t.Fatalf("name of %d bytes should fit: %v", len(fits), err)
	}
	err := b.Create(context.Background(), core.Environment{Name: fits + "a"})
	if !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("Create with a too-long name = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("at most %d bytes", len(fits))) {
		t.Fatalf("error should state the limit of %d bytes: %v", len(fits), err)
	}
}

func TestDisplayArgs(t *testing.T) {
	const memfd = " -object memory-backend-memfd,id=mem,size=2048M,share=on -machine memory-backend=mem"
	tests := map[string]struct {
		openGL, vulkan, virtiofs bool
		want                     string
	}{
		"no host gpu":   {false, false, false, "-device virtio-vga -display dbus,p2p=yes"},
		"no gpu, fs":    {false, false, true, "-device virtio-vga -display dbus,p2p=yes" + memfd},
		"vulkan w/o gl": {false, true, false, "-device virtio-vga -display dbus,p2p=yes"},
		"opengl":        {true, false, false, "-device virtio-vga-gl -display dbus,p2p=yes,gl=on"},
		"opengl, fs":    {true, false, true, "-device virtio-vga-gl -display dbus,p2p=yes,gl=on" + memfd},
		"vulkan":        {true, true, false, "-device virtio-vga-gl,blob=on,hostmem=4G,venus=on -display dbus,p2p=yes,gl=on" + memfd},
	}
	for name, tt := range tests {
		if got := strings.Join(displayArgs(tt.openGL, tt.vulkan, 2048, tt.virtiofs), " "); got != tt.want {
			t.Errorf("%s: got %q, want %q", name, got, tt.want)
		}
	}
}

func TestNetworkIsExplicitWithAStableMACPerMachine(t *testing.T) {
	a := core.Environment{ID: "aaaaaaaaaaaaaaaa"}
	b := core.Environment{ID: "bbbbbbbbbbbbbbbb"}
	first := networkArgs(a, true)
	if got := strings.Join(first, " "); !strings.HasPrefix(got, "-nic user,model=e1000e,mac=52:54:00:") {
		t.Errorf("q35 network = %q", got)
	}
	if got := strings.Join(networkArgs(a, false), " "); !strings.Contains(got, "model=e1000,") {
		t.Errorf("pc network = %q", got)
	}
	if !slices.Equal(first, networkArgs(a, true)) {
		t.Error("the MAC changed between starts")
	}
	if slices.Equal(first, networkArgs(b, true)) {
		t.Error("two Machines (a clone) share a MAC")
	}
}

// A stopped Machine shows the last frame of its last session, when there
// is one; never a placeholder pretending it ran.
func TestPreviewOfAStoppedMachineIsItsLastFrame(t *testing.T) {
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{ID: "aaaaaaaaaaaaaaaa", Name: "vm", Kind: core.Machine}
	if _, err := b.Preview(context.Background(), env); !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("a Machine that never ran has no preview, got %v", err)
	}
	frame := b.previewPath(b.key(env))
	if err := os.MkdirAll(filepath.Dir(frame), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(frame, []byte("P6\n1 1\n255\n\x00\x00\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Preview(context.Background(), env); err != nil || got != frame {
		t.Errorf("Preview = %q, %v; want the saved frame %q", got, err, frame)
	}
}

// Every Machine shares ~/OmaVM/Shared unless Settings choose another
// folder or turn sharing off.
func TestMachineSettingsResolveTheSharedFolder(t *testing.T) {
	env := core.Environment{Kind: core.Machine}
	if got := machineSettings(env).SharedPath; got != defaultSharedDir() || !strings.HasSuffix(got, "/OmaVM/Shared") {
		t.Errorf("default shared folder = %q", got)
	}
	env.Settings.SharedPath = "/srv/work"
	if got := machineSettings(env).SharedPath; got != "/srv/work" {
		t.Errorf("chosen shared folder = %q", got)
	}
	env.Settings.SharedFolderDisabled = true
	if got := machineSettings(env).SharedPath; got != "" {
		t.Errorf("sharing turned off still shares %q", got)
	}
}
