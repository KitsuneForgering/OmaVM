package qemu

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// firmwareDirs hold QEMU's firmware descriptors (docs/interop/firmware.json
// in QEMU), the same files libvirt reads: each distro says where its OVMF
// build lives and what it supports, so nothing here hardcodes Arch's
// paths. A file in a later directory replaces one with the same name in an
// earlier one; overridable in tests.
var firmwareDirs = func() []string {
	dirs := []string{"/usr/share/qemu/firmware", "/etc/qemu/firmware"}
	if config, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(config, "qemu", "firmware"))
	}
	return dirs
}()

// firmware is the UEFI build a Machine boots, recorded in its state
// directory when it is created (firmware.json). A Machine without that
// file was created before UEFI, or on a host without OVMF, and keeps
// booting with QEMU's default BIOS: switching an installed system's
// firmware would leave it unbootable.
type firmware struct {
	Executable    string `json:"executable"`
	Format        string `json:"format"`
	NVRAMFormat   string `json:"nvram_format"`
	SecureBoot    bool   `json:"secure_boot"`
	RequiresSMM   bool   `json:"requires_smm"`
	nvramTemplate string
}

type firmwareDescriptor struct {
	InterfaceTypes []string `json:"interface-types"`
	Mapping        struct {
		Device     string `json:"device"`
		Mode       string `json:"mode"`
		Executable struct {
			Filename string `json:"filename"`
			Format   string `json:"format"`
		} `json:"executable"`
		NVRAMTemplate struct {
			Filename string `json:"filename"`
			Format   string `json:"format"`
		} `json:"nvram-template"`
	} `json:"mapping"`
	Targets []struct {
		Architecture string   `json:"architecture"`
		Machines     []string `json:"machines"`
	} `json:"targets"`
	Features []string `json:"features"`
}

// findFirmware picks the first descriptor, in file name order as QEMU's
// specification asks, for UEFI on x86_64 q35 with a separate NVRAM
// template (each Machine needs its own variables). Distros name their
// Secure Boot builds first (Arch: 50-…-secure, 60-… plain), so the first
// match is the most capable one. Builds for confidential guests (SEV,
// TDX) have no NVRAM template and are never chosen.
func findFirmware() (firmware, bool) {
	files := map[string]string{}
	for _, dir := range firmwareDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".json" {
				files[entry.Name()] = filepath.Join(dir, entry.Name())
			}
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := os.ReadFile(files[name])
		if err != nil {
			continue
		}
		var d firmwareDescriptor
		if json.Unmarshal(data, &d) != nil || !usableFirmware(d) {
			continue
		}
		return firmware{
			Executable:    d.Mapping.Executable.Filename,
			Format:        d.Mapping.Executable.Format,
			NVRAMFormat:   d.Mapping.NVRAMTemplate.Format,
			SecureBoot:    slices.Contains(d.Features, "secure-boot"),
			RequiresSMM:   slices.Contains(d.Features, "requires-smm"),
			nvramTemplate: d.Mapping.NVRAMTemplate.Filename,
		}, true
	}
	return firmware{}, false
}

func usableFirmware(d firmwareDescriptor) bool {
	m := d.Mapping
	if !slices.Contains(d.InterfaceTypes, "uefi") || m.Device != "flash" || (m.Mode != "" && m.Mode != "split") {
		return false
	}
	if m.Executable.Filename == "" || m.NVRAMTemplate.Filename == "" {
		return false
	}
	for _, format := range []string{m.Executable.Format, m.NVRAMTemplate.Format} {
		if format != "raw" && format != "qcow2" {
			return false
		}
	}
	for _, file := range []string{m.Executable.Filename, m.NVRAMTemplate.Filename} {
		if _, err := os.Stat(file); err != nil {
			return false
		}
	}
	for _, t := range d.Targets {
		if t.Architecture != "x86_64" {
			continue
		}
		for _, pattern := range t.Machines {
			if ok, _ := path.Match(pattern, "pc-q35-11.0"); ok {
				return true
			}
		}
	}
	return false
}

func (b *Backend) firmwarePath(name string) string {
	return filepath.Join(b.dir(name), "firmware.json")
}
func (b *Backend) nvramPath(name string) string { return filepath.Join(b.dir(name), "efivars.fd") }

// setUpFirmware gives a new Machine UEFI: its own copy of the NVRAM
// template, then the record that says which build it belongs to (written
// last, so a Machine never claims UEFI without its variables). Without
// OVMF on the host the Machine is created with BIOS, which still installs
// most systems; omavm host says what is missing.
func (b *Backend) setUpFirmware(name string) error {
	fw, ok := findFirmware()
	if !ok {
		slog.Warn("no UEFI firmware found; the machine will use BIOS", "machine", name, "hint", "install edk2-ovmf")
		return nil
	}
	if err := copyFile(fw.nvramTemplate, b.nvramPath(name), 0o600); err != nil {
		return fmt.Errorf("copy UEFI variables: %w", err)
	}
	data, err := json.Marshal(fw)
	if err != nil {
		return err
	}
	if err := os.WriteFile(b.firmwarePath(name), data, 0o600); err != nil {
		_ = os.Remove(b.nvramPath(name))
		return fmt.Errorf("record UEFI firmware: %w", err)
	}
	return nil
}

// machineFirmware reads a Machine's UEFI record; ok is false for a BIOS
// Machine.
func (b *Backend) machineFirmware(name string) (fw firmware, ok bool, err error) {
	data, err := os.ReadFile(b.firmwarePath(name))
	if errors.Is(err, os.ErrNotExist) {
		return firmware{}, false, nil
	}
	if err != nil {
		return firmware{}, false, err
	}
	if err := json.Unmarshal(data, &fw); err != nil {
		return firmware{}, false, fmt.Errorf("read %s: %w", b.firmwarePath(name), err)
	}
	return fw, true, nil
}

// firmwareArgs boots a UEFI Machine: q35 (Secure Boot needs SMM, which
// the default pc machine has no way to protect), the read-only firmware
// and the Machine's own variables. A BIOS Machine gets no arguments and
// keeps QEMU's defaults.
func (b *Backend) firmwareArgs(name string) ([]string, error) {
	fw, ok, err := b.machineFirmware(name)
	if err != nil || !ok {
		return nil, err
	}
	if _, err := os.Stat(fw.Executable); err != nil {
		return nil, fmt.Errorf("this Machine boots with UEFI, but its firmware %s is missing (install it with: sudo pacman -S edk2-ovmf)", fw.Executable)
	}
	machine := "q35"
	if fw.RequiresSMM {
		machine += ",smm=on"
	}
	args := []string{"-machine", machine}
	if fw.RequiresSMM {
		args = append(args, "-global", "driver=cfi.pflash01,property=secure,value=on")
	}
	return append(args,
		"-drive", fmt.Sprintf("if=pflash,format=%s,unit=0,readonly=on,file=%s", fw.Format, fw.Executable),
		"-drive", fmt.Sprintf("if=pflash,format=%s,unit=1,file=%s", fw.NVRAMFormat, b.nvramPath(name)),
	), nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func firmwareCapability() core.HostCapability {
	c := core.HostCapability{ID: "uefi", Label: "UEFI firmware for new Desktops"}
	fw, ok := findFirmware()
	switch {
	case !ok:
		c.Detail = "no UEFI firmware (OVMF) found, so new Desktops use BIOS and can't install Windows 11"
		c.Hint = "Install edk2-ovmf"
	case fw.SecureBoot:
		c.Available = true
		c.Detail = "with Secure Boot (" + fw.Executable + "); Desktops created before keep the firmware they were installed with"
	default:
		c.Available = true
		c.Detail = "without Secure Boot (" + fw.Executable + "), which Windows 11 requires"
	}
	return c
}
