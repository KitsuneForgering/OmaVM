package qemu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A Machine's disk bus is chosen once and then kept: a system installed
// on one bus may not boot from another (an initramfs built without that
// driver, a Windows without it).
//
// New Machines get NVMe, which every guest OmaVM targets drives with no
// extra drivers: Windows 10/11's installer, Linux, the BSDs, Haiku.
// virtio-blk needs virtio-win on Windows, and Haiku r1beta6 doesn't see
// it at all (checked 2026-10-07 in its DriveSetup: SATA and NVMe disks
// listed, a virtio one missing). Machines from before keep virtio, unless
// their disk was never written: then nothing installed can break.
const (
	busNVMe   = "nvme"
	busVirtio = "virtio"
)

func (b *Backend) diskBusPath(name string) string {
	return filepath.Join(b.dir(name), "disk-bus")
}

// recordedDiskBus is the bus this Machine was given, or "" before its
// first Start.
func (b *Backend) recordedDiskBus(name string) string {
	data, err := os.ReadFile(b.diskBusPath(name))
	if err != nil {
		return ""
	}
	switch bus := strings.TrimSpace(string(data)); bus {
	case busNVMe, busVirtio:
		return bus
	}
	return ""
}

// diskBus returns the Machine's bus, deciding and recording it the first
// time. A disk that can't be inspected starts on virtio, unrecorded:
// asked again next time, it can only turn NVMe while still empty, so no
// installed system ever changes bus. NVMe must be recorded before it is
// used, or a later Start would pick virtio for the disk installed on it.
func (b *Backend) diskBus(ctx context.Context, name string) (string, error) {
	if bus := b.recordedDiskBus(name); bus != "" {
		return bus, nil
	}
	empty, err := diskNeverWritten(ctx, b.diskPath(name))
	if err != nil {
		slog.Warn("disk not inspected; starting it on virtio", "machine", name, "error", err)
		return busVirtio, nil
	}
	bus := busVirtio
	if empty {
		bus = busNVMe
	}
	if err := os.WriteFile(b.diskBusPath(name), []byte(bus+"\n"), 0o600); err != nil {
		if bus == busNVMe {
			return "", fmt.Errorf("record the disk's bus: %w", err)
		}
		slog.Warn("disk bus not recorded", "machine", name, "error", err)
	}
	return bus, nil
}

// diskNeverWritten reports whether a qcow2 holds no data at all: nothing
// written by a guest, and no snapshots that could hold some.
func diskNeverWritten(ctx context.Context, path string) (bool, error) {
	out, err := qemuImgJSON(ctx, "info", "-U", "--output=json", path)
	if err != nil {
		return false, err
	}
	var info struct {
		Snapshots []json.RawMessage `json:"snapshots"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return false, fmt.Errorf("inspect the disk: %w", err)
	}
	if len(info.Snapshots) > 0 {
		return false, nil
	}
	out, err = qemuImgJSON(ctx, "map", "-U", "--output=json", path)
	if err != nil {
		return false, err
	}
	var extents []struct {
		Data bool `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &extents); err != nil {
		return false, fmt.Errorf("inspect the disk: %w", err)
	}
	for _, e := range extents {
		if e.Data {
			return false, nil
		}
	}
	return true, nil
}

// qemuImgJSON runs qemu-img for its JSON on stdout; warnings on stderr
// are kept out of it.
func qemuImgJSON(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "qemu-img", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("inspect the disk: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// diskArgs attaches the disk on its bus. NVMe is boot entry 0, ahead of
// the installation media (SeaBIOS's boot menu lists it first; OVMF falls
// back to the ISO while the disk is empty). discard=unmap: space the
// guest frees (fstrim, Windows' Optimize Drives) goes back to the host;
// QEMU's default ignores it. Clusters a snapshot still uses stay, by
// qcow2's own reference counts.
func diskArgs(path, bus string) []string {
	if bus == busNVMe {
		return []string{
			"-drive", fmt.Sprintf("file=%s,if=none,id=%s,format=qcow2,discard=unmap", optValue(path), diskDevice(bus)),
			"-device", "nvme,drive=" + diskDevice(bus) + ",serial=omavm-disk,bootindex=0",
		}
	}
	return []string{"-drive", fmt.Sprintf("file=%s,if=virtio,format=qcow2,discard=unmap", optValue(path))}
}

// diskDevice is the name QMP knows the disk by: QEMU's own for if=virtio,
// the drive's id for NVMe.
func diskDevice(bus string) string {
	if bus == busNVMe {
		return "disk0"
	}
	return "virtio0"
}
