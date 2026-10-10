package qemu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// Tests never see the host's OVMF or swtpm: each one that needs them sets
// up its own. Nor its home: a Start creates the default shared folder
// (~/OmaVM/Shared) and links land in ~/OmaVM.
func TestMain(m *testing.M) {
	firmwareDirs = nil
	swtpmLookPath = func() (string, error) { return "", exec.ErrNotFound }
	quickgetLookPath = func() (string, error) { return "", exec.ErrNotFound }
	// The real virtiofsd waits for QEMU to connect, forever: with a fake
	// QEMU, every test that started a Machine left one running (14 per
	// run, 258 on this machine by 2026-10-08). Tests that share a folder
	// use a fake virtiofsd (virtiofs_test.go).
	virtiofsdPath = func() (string, error) { return "", exec.ErrNotFound }
	home, err := os.MkdirTemp("", "omavm-qemu-home")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	if leaked := processesMentioning(home); len(leaked) > 0 && code == 0 {
		fmt.Fprintf(os.Stderr, "tests left processes running:\n%s\n", strings.Join(leaked, "\n"))
		code = 1
	}
	os.RemoveAll(home)
	os.Exit(code)
}

// processesMentioning lists the command lines of this user's processes
// that name path: helpers (virtiofsd, swtpm, QEMU) a test started and
// never stopped.
func processesMentioning(path string) []string {
	var found []string
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil || entry.Name() == strconv.Itoa(os.Getpid()) {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || !bytes.Contains(cmdline, []byte(path)) {
			continue
		}
		found = append(found, entry.Name()+": "+strings.ReplaceAll(string(cmdline), "\x00", " "))
	}
	return found
}

const secureDescriptor = `{
  "interface-types": ["uefi"],
  "mapping": {"device": "flash",
    "executable": {"filename": "%DIR%/CODE.secboot.fd", "format": "raw"},
    "nvram-template": {"filename": "%DIR%/VARS.fd", "format": "raw"}},
  "targets": [{"architecture": "x86_64", "machines": ["pc-q35-*"]}],
  "features": ["acpi-s3", "requires-smm", "secure-boot"]
}`

const plainDescriptor = `{
  "interface-types": ["uefi"],
  "mapping": {"device": "flash",
    "executable": {"filename": "%DIR%/CODE.fd", "format": "raw"},
    "nvram-template": {"filename": "%DIR%/VARS.fd", "format": "raw"}},
  "targets": [{"architecture": "x86_64", "machines": ["pc-i440fx-*", "pc-q35-*"]}],
  "features": ["acpi-s3"]
}`

// Distros also ship builds that must never be picked: confidential-guest
// firmware with no NVRAM template, other architectures, BIOS, and
// descriptors whose files were removed.
var unusableDescriptors = map[string]string{
	"10-sev.json": `{"interface-types": ["uefi"], "mapping": {"device": "flash", "mode": "stateless",
	  "executable": {"filename": "%DIR%/CODE.fd", "format": "raw"}},
	  "targets": [{"architecture": "x86_64", "machines": ["pc-q35-*"]}], "features": ["amd-sev"]}`,
	"11-aarch64.json": `{"interface-types": ["uefi"], "mapping": {"device": "flash",
	  "executable": {"filename": "%DIR%/CODE.fd", "format": "raw"},
	  "nvram-template": {"filename": "%DIR%/VARS.fd", "format": "raw"}},
	  "targets": [{"architecture": "aarch64", "machines": ["virt-*"]}]}`,
	"12-bios.json": `{"interface-types": ["bios"], "mapping": {"device": "memory", "filename": "%DIR%/bios.bin"},
	  "targets": [{"architecture": "x86_64", "machines": ["pc-q35-*"]}]}`,
	"13-gone.json": `{"interface-types": ["uefi"], "mapping": {"device": "flash",
	  "executable": {"filename": "%DIR%/missing.fd", "format": "raw"},
	  "nvram-template": {"filename": "%DIR%/VARS.fd", "format": "raw"}},
	  "targets": [{"architecture": "x86_64", "machines": ["pc-q35-*"]}]}`,
	"14-broken.json": `{not json`,
}

// useFirmware installs descriptors (name -> JSON with %DIR% for the
// firmware directory) and the firmware files they point to.
func useFirmware(t *testing.T, descriptors map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for _, file := range []string{"CODE.secboot.fd", "CODE.fd", "bios.bin"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("code"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "VARS.fd"), []byte("empty variables"), 0o644); err != nil {
		t.Fatal(err)
	}
	descDir := filepath.Join(dir, "descriptors")
	if err := os.Mkdir(descDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range descriptors {
		if err := os.WriteFile(filepath.Join(descDir, name), []byte(strings.ReplaceAll(body, "%DIR%", dir)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	saved := firmwareDirs
	firmwareDirs = []string{descDir}
	t.Cleanup(func() { firmwareDirs = saved })
	return dir
}

func TestFindFirmwarePicksTheFirstUsableDescriptor(t *testing.T) {
	descriptors := map[string]string{
		"50-secure.json": secureDescriptor,
		"60-plain.json":  plainDescriptor,
	}
	for name, body := range unusableDescriptors {
		descriptors[name] = body
	}
	dir := useFirmware(t, descriptors)
	fw, ok := findFirmware()
	if !ok {
		t.Fatal("no firmware found")
	}
	if fw.Executable != filepath.Join(dir, "CODE.secboot.fd") || !fw.SecureBoot || !fw.RequiresSMM || fw.nvramTemplate != filepath.Join(dir, "VARS.fd") {
		t.Fatalf("picked %+v", fw)
	}
}

func TestFindFirmwareHonorsOverridesAndAbsence(t *testing.T) {
	dir := useFirmware(t, map[string]string{"50-secure.json": secureDescriptor, "60-plain.json": plainDescriptor})
	// The user's directory replaces a descriptor of the same name, here
	// with one that is no use, so the next one is picked.
	user := t.TempDir()
	if err := os.WriteFile(filepath.Join(user, "50-secure.json"), []byte(unusableDescriptors["11-aarch64.json"]), 0o644); err != nil {
		t.Fatal(err)
	}
	firmwareDirs = append(firmwareDirs, user)
	if fw, ok := findFirmware(); !ok || fw.Executable != filepath.Join(dir, "CODE.fd") || fw.SecureBoot {
		t.Fatalf("override ignored: %+v %v", fw, ok)
	}

	firmwareDirs = []string{t.TempDir()}
	if fw, ok := findFirmware(); ok {
		t.Fatalf("found firmware with no descriptors: %+v", fw)
	}
}

// fakeQEMUImg creates the disk qemu-img would, next to the fake
// qemu-system-x86_64 in the same PATH directory.
func fakeQEMUImg(t *testing.T, binDir string) {
	t.Helper()
	script := "#!/bin/sh\n[ \"$1\" = create ] && : > \"$4\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "qemu-img"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestCreateGivesANewMachineUEFI(t *testing.T) {
	fwDir := useFirmware(t, map[string]string{"50-secure.json": secureDescriptor})
	bin, _ := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{ID: "0123456789abcdef", Name: "win", Kind: core.Machine}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	name := b.key(env)
	fw, uefi, err := b.machineFirmware(name)
	if err != nil || !uefi || fw.Executable != filepath.Join(fwDir, "CODE.secboot.fd") {
		t.Fatalf("firmware record: %+v uefi=%v err=%v", fw, uefi, err)
	}
	vars, err := os.ReadFile(b.nvramPath(name))
	if err != nil || string(vars) != "empty variables" {
		t.Fatalf("NVRAM is not a copy of the template: %q %v", vars, err)
	}
	if info, _ := os.Stat(b.nvramPath(name)); info.Mode().Perm() != 0o600 {
		t.Errorf("NVRAM mode %v, want 0600", info.Mode().Perm())
	}
}

func TestCreateWithoutOVMFFallsBackToBIOS(t *testing.T) {
	bin, _ := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "old", Kind: core.Machine}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if _, uefi, err := b.machineFirmware(b.key(env)); uefi || err != nil {
		t.Fatalf("uefi=%v err=%v without any firmware on the host", uefi, err)
	}
}

// An existing Machine was installed with BIOS: OVMF appearing on the host
// later must never switch it, or its system would no longer boot.
func TestExistingMachineKeepsBIOS(t *testing.T) {
	useFirmware(t, map[string]string{"50-secure.json": secureDescriptor})
	bin, log := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	useVsock(t, false)
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "installed", Kind: core.Machine}
	if err := os.MkdirAll(b.dir(b.key(env)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.diskPath(b.key(env)), []byte("installed system"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(log)
	if strings.Contains(string(data), "pflash") || strings.Contains(string(data), "q35") {
		t.Fatalf("a BIOS Machine started with UEFI: %s", data)
	}
}

func TestStartBootsAUEFIMachineWithSecureBoot(t *testing.T) {
	fwDir := useFirmware(t, map[string]string{"50-secure.json": secureDescriptor})
	bin, log := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	useVsock(t, false)
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "win", Kind: core.Machine}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(log)
	for _, want := range []string{
		"-machine q35,smm=on",
		"-global driver=cfi.pflash01,property=secure,value=on",
		"if=pflash,format=raw,unit=0,readonly=on,file=" + filepath.Join(fwDir, "CODE.secboot.fd"),
		"if=pflash,format=raw,unit=1,file=" + b.nvramPath(b.key(env)),
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q in:\n%s", want, data)
		}
	}
}

func TestStartWithoutTheMachinesFirmwareSaysWhatToInstall(t *testing.T) {
	fwDir := useFirmware(t, map[string]string{"60-plain.json": plainDescriptor})
	bin, log := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "win", Kind: core.Machine}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fwDir, "CODE.fd")); err != nil {
		t.Fatal(err)
	}
	err := b.Start(context.Background(), env)
	if err == nil || !strings.Contains(err.Error(), "edk2-ovmf") {
		t.Fatalf("expected an install hint, got %v", err)
	}
	if data, _ := os.ReadFile(log); strings.Contains(string(data), "-name") {
		t.Fatalf("QEMU ran without its firmware: %s", data)
	}
}

func TestCloneTakesTheFirmwareAndTPMAlong(t *testing.T) {
	useFirmware(t, map[string]string{"50-secure.json": secureDescriptor})
	bin, _ := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	t.Setenv("PATH", bin+":/usr/bin:/bin") // cp
	b := &Backend{stateDir: t.TempDir()}
	src := core.Environment{ID: "aaaaaaaaaaaaaaaa", Name: "win", Kind: core.Machine}
	dst := core.Environment{ID: "bbbbbbbbbbbbbbbb", Name: "win copy", Kind: core.Machine}
	if err := b.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b.tpmStateDir(b.key(src)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.tpmStateDir(b.key(src)), "tpm2-00.permall"), []byte("sealed keys"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.Clone(context.Background(), src, dst); err != nil {
		t.Fatal(err)
	}
	name := b.key(dst)
	if _, uefi, _ := b.machineFirmware(name); !uefi {
		t.Error("the clone lost its UEFI record")
	}
	if _, err := os.Stat(b.nvramPath(name)); err != nil {
		t.Errorf("the clone has no UEFI variables: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(b.tpmStateDir(name), "tpm2-00.permall"))
	if err != nil || string(data) != "sealed keys" {
		t.Errorf("the clone's TPM state: %q %v", data, err)
	}
	if info, err := os.Stat(b.tpmStateDir(name)); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the clone's TPM directory must stay private: %v %v", info.Mode(), err)
	}
}

func TestCloneOfABIOSMachineCopiesOnlyTheDisk(t *testing.T) {
	bin, _ := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	b := &Backend{stateDir: t.TempDir()}
	src := core.Environment{ID: "aaaaaaaaaaaaaaaa", Name: "old", Kind: core.Machine}
	dst := core.Environment{ID: "bbbbbbbbbbbbbbbb", Name: "old copy", Kind: core.Machine}
	if err := b.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if err := b.Clone(context.Background(), src, dst); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{b.firmwarePath(b.key(dst)), b.nvramPath(b.key(dst)), b.tpmStateDir(b.key(dst))} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s exists for a BIOS clone: %v", path, err)
		}
	}
}

func TestSnapshotOfAStoppedMachineKeepsItsFirmwareState(t *testing.T) {
	useFirmware(t, map[string]string{"50-secure.json": secureDescriptor})
	bin, _ := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	t.Setenv("PATH", bin+":/usr/bin:/bin") // cp
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{ID: "aaaaaaaaaaaaaaaa", Name: "win", Kind: core.Machine}
	ctx := context.Background()
	if err := b.Create(ctx, env); err != nil {
		t.Fatal(err)
	}
	name := b.key(env)
	keys := filepath.Join(b.tpmStateDir(name), "tpm2-00.permall")
	write := func(nvram, tpm string) {
		t.Helper()
		if err := os.MkdirAll(b.tpmStateDir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(b.nvramPath(name), []byte(nvram), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keys, []byte(tpm), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("boot entries 1", "sealed 1")
	if _, err := b.CreateSnapshot(ctx, env, "clean-1"); err != nil {
		t.Fatal(err)
	}
	write("boot entries 2", "sealed 2")
	if err := b.GoToSnapshot(ctx, env, "clean-1"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{b.nvramPath(name): "boot entries 1", keys: "sealed 1"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Errorf("%s = %q %v, want %q", filepath.Base(path), data, err, want)
		}
	}
	if info, err := os.Stat(b.tpmStateDir(name)); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the restored TPM directory must stay private: %v", err)
	}
	if err := b.RemoveSnapshot(ctx, env, "clean-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.platformSnapshotDir(name, "clean-1")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the snapshot's firmware state outlived it: %v", err)
	}
}
