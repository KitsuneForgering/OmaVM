package qemu

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// newDisk makes a real qcow2 for the bus decision, which reads it with
// qemu-img; skipped without it.
func newDisk(t *testing.T, b *Backend, name string) {
	t.Helper()
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("needs qemu-img")
	}
	if err := os.MkdirAll(b.dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("qemu-img", "create", "-q", "-f", "qcow2", b.diskPath(name), "64M").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func qemuIO(t *testing.T, path, command string) {
	t.Helper()
	if out, err := exec.Command("qemu-io", "-f", "qcow2", "-c", command, path).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// A Machine whose disk was never written gets NVMe, and keeps it once a
// system is installed on it.
func TestEmptyDiskGetsNVMeForGood(t *testing.T) {
	b := &Backend{stateDir: t.TempDir()}
	newDisk(t, b, "m")
	if bus, err := b.diskBus(context.Background(), "m"); err != nil || bus != busNVMe {
		t.Fatalf("got %q, %v; want nvme", bus, err)
	}
	qemuIO(t, b.diskPath("m"), "write 0 64k")
	if bus, err := b.diskBus(context.Background(), "m"); err != nil || bus != busNVMe {
		t.Fatalf("an installed NVMe disk changed to %q (%v)", bus, err)
	}
}

// Machines from before NVMe keep virtio: the system on them may not boot
// from another bus. Data in a snapshot counts as much as data on the disk.
func TestWrittenDiskKeepsVirtio(t *testing.T) {
	for name, write := range map[string]func(t *testing.T, path string){
		"data": func(t *testing.T, path string) { qemuIO(t, path, "write 1M 4k") },
		"snapshot": func(t *testing.T, path string) {
			if out, err := exec.Command("qemu-img", "snapshot", "-c", "before", path).CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := &Backend{stateDir: t.TempDir()}
			newDisk(t, b, "m")
			write(t, b.diskPath("m"))
			if bus, err := b.diskBus(context.Background(), "m"); err != nil || bus != busVirtio {
				t.Fatalf("got %q, %v; want virtio", bus, err)
			}
			if got := b.recordedDiskBus("m"); got != busVirtio {
				t.Fatalf("recorded %q, want virtio", got)
			}
		})
	}
}

// Without a disk to inspect, nothing is recorded: deciding later can only
// pick NVMe for a disk still empty then.
func TestUninspectableDiskStartsOnVirtioUnrecorded(t *testing.T) {
	b := &Backend{stateDir: t.TempDir()}
	if err := os.MkdirAll(b.dir("m"), 0o700); err != nil {
		t.Fatal(err)
	}
	if bus, err := b.diskBus(context.Background(), "m"); err != nil || bus != busVirtio {
		t.Fatalf("got %q, %v; want virtio", bus, err)
	}
	if got := b.recordedDiskBus("m"); got != "" {
		t.Fatalf("recorded %q for a disk that wasn't inspected", got)
	}
}

func TestDiskArgs(t *testing.T) {
	nvme := strings.Join(diskArgs("/s/a,b/disk.qcow2", busNVMe), " ")
	for _, want := range []string{"file=/s/a,,b/disk.qcow2,if=none,id=disk0", "discard=unmap", "nvme,drive=disk0", "bootindex=0"} {
		if !strings.Contains(nvme, want) {
			t.Errorf("NVMe args %q lack %q", nvme, want)
		}
	}
	// Unchanged for Machines that keep virtio: QEMU names it virtio0.
	if got := strings.Join(diskArgs("/s/disk.qcow2", busVirtio), " "); got != "-drive file=/s/disk.qcow2,if=virtio,format=qcow2,discard=unmap" {
		t.Errorf("virtio args changed: %q", got)
	}
	if diskDevice(busNVMe) != "disk0" || diskDevice(busVirtio) != "virtio0" || diskDevice("") != "virtio0" {
		t.Error("QMP device names don't match the drives")
	}
}

// A clone holds the same installed system, so it boots on the same bus.
func TestCloneKeepsTheDiskBus(t *testing.T) {
	b := &Backend{stateDir: t.TempDir()}
	source := core.Environment{ID: "src", Name: "vm", Kind: core.Machine}
	clone := core.Environment{ID: "dst", Name: "vm copy", Kind: core.Machine}
	if err := os.MkdirAll(b.dir("src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.diskPath("src"), []byte("qcow2 bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.diskBusPath("src"), []byte("nvme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.Clone(context.Background(), source, clone); err != nil {
		t.Fatal(err)
	}
	if got := b.recordedDiskBus("dst"); got != busNVMe {
		t.Fatalf("clone's bus %q, want nvme", got)
	}
}

// The viewer waits out the firmware only for a Machine that boots an
// installed system: an empty disk boots the installation media, whose
// prompts must be seen.
func TestViewerWaitsOutTheFirmwareOnlyForAnInstalledSystem(t *testing.T) {
	b := &Backend{stateDir: t.TempDir()}
	newDisk(t, b, "m")
	if b.opensAfterFirmware(context.Background(), "m") {
		t.Fatal("an empty disk (installing from the ISO) must show the firmware")
	}
	qemuIO(t, b.diskPath("m"), "write 0 64k")
	if !b.opensAfterFirmware(context.Background(), "m") {
		t.Fatal("an installed system should open after its firmware")
	}
	if got := strings.Join(viewerArgs(core.Environment{Name: "vm", Kind: core.Machine}, "", true), " "); !strings.Contains(got, "--reveal-after-firmware") {
		t.Fatalf("viewer args %q lack --reveal-after-firmware", got)
	}
	if got := strings.Join(viewerArgs(core.Environment{Name: "vm", Kind: core.Machine}, "", false), " "); strings.Contains(got, "--reveal-after-firmware") {
		t.Fatalf("viewer args %q wait for the firmware unasked", got)
	}
}
