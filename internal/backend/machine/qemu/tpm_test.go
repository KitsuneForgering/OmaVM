package qemu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// fakeSwtpm stands in for swtpm: it records its arguments, creates the
// control socket path it was given (a plain file is enough for Start to
// see it) and waits to be stopped. With fail set it exits at once with
// swtpm-like output instead.
func fakeSwtpm(t *testing.T, fail bool) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "swtpm-args")
	script := "#!/bin/sh\necho \"$*\" > " + log + "\n"
	if fail {
		script += "echo 'swtpm: Could not open TPM state directory' >&2\nexit 1\n"
	} else {
		script += "for a in \"$@\"; do case \"$a\" in type=unixio,path=*) : > \"${a#type=unixio,path=}\";; esac; done\n" +
			"trap 'kill $! 2>/dev/null; exit 0' TERM\nsleep 30 & wait\n"
	}
	path := filepath.Join(dir, "swtpm")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	saved := swtpmLookPath
	swtpmLookPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { swtpmLookPath = saved })
	return log
}

// uefiMachine creates a UEFI Machine with fake QEMU binaries and returns
// it with the log of QEMU's runs.
func uefiMachine(t *testing.T) (*Backend, core.Environment, string) {
	t.Helper()
	useFirmware(t, map[string]string{"50-secure.json": secureDescriptor})
	bin, log := fakeQEMU(t, false)
	fakeQEMUImg(t, bin)
	t.Setenv("PATH", bin+":/usr/bin:/bin") // sh, sleep, cp for the fakes
	useVsock(t, false)
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{ID: "0123456789abcdef", Name: "win", Kind: core.Machine}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	return b, env, log
}

func TestStartGivesAUEFIMachineATPM(t *testing.T) {
	b, env, log := uefiMachine(t)
	swtpmLog := fakeSwtpm(t, false)
	if err := b.Start(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.stopTPM(b.key(env)) })
	name := b.key(env)

	args, _ := os.ReadFile(swtpmLog)
	for _, want := range []string{"socket --tpm2", "--tpmstate dir=" + b.tpmStateDir(name) + ",mode=0600", "type=unixio,path=" + b.tpmSocketPath(name), "--terminate"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("swtpm: missing %q in %s", want, args)
		}
	}
	if info, err := os.Stat(b.tpmStateDir(name)); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("TPM state must be private to this user: %v %v", info, err)
	}
	runs, _ := os.ReadFile(log)
	for _, want := range []string{"-chardev socket,id=tpm,path=" + b.tpmSocketPath(name), "-tpmdev emulator,id=tpm0,chardev=tpm", "-device tpm-crb,tpmdev=tpm0"} {
		if !strings.Contains(string(runs), want) {
			t.Errorf("QEMU: missing %q in %s", want, runs)
		}
	}
}

// A BIOS Machine has no TPM: Windows 11, the reason for it, needs UEFI
// anyway, and swtpm is not even looked for.
func TestBIOSMachineHasNoTPM(t *testing.T) {
	bin, log := fakeQEMU(t, false)
	useVsock(t, false)
	_ = bin
	swtpmLog := fakeSwtpm(t, false)
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "old", Kind: core.Machine}
	if err := b.Start(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(swtpmLog); err == nil {
		t.Error("swtpm ran for a BIOS Machine")
	}
	if runs, _ := os.ReadFile(log); strings.Contains(string(runs), "tpm") {
		t.Errorf("BIOS Machine got a TPM: %s", runs)
	}
}

func TestStartWithoutSwtpm(t *testing.T) {
	t.Run("unused", func(t *testing.T) {
		b, env, log := uefiMachine(t)
		if err := b.Start(context.Background(), env); err != nil {
			t.Fatal(err)
		}
		if runs, _ := os.ReadFile(log); strings.Contains(string(runs), "tpm-crb") {
			t.Errorf("TPM device without swtpm: %s", runs)
		}
	})
	t.Run("in use", func(t *testing.T) {
		b, env, log := uefiMachine(t)
		state := b.tpmStateDir(b.key(env))
		if err := os.MkdirAll(state, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(state, "tpm2-00.permall"), []byte("sealed"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := b.Start(context.Background(), env)
		if err == nil || !strings.Contains(err.Error(), "swtpm") {
			t.Fatalf("expected an install hint, got %v", err)
		}
		if runs, _ := os.ReadFile(log); strings.Contains(string(runs), "-name") {
			t.Errorf("QEMU ran without the TPM that holds its keys: %s", runs)
		}
	})
}

func TestSwtpmFailureStopsTheStart(t *testing.T) {
	b, env, log := uefiMachine(t)
	fakeSwtpm(t, true)
	err := b.Start(context.Background(), env)
	if err == nil || !strings.Contains(err.Error(), "Could not open TPM state directory") {
		t.Fatalf("expected swtpm's own words, got %v", err)
	}
	if runs, _ := os.ReadFile(log); strings.Contains(string(runs), "-name") {
		t.Errorf("QEMU ran without its TPM: %s", runs)
	}
}

// A session that keeps no changes must not keep the TPM's either: swtpm
// works on a copy, gone once the Machine is stopped.
func TestEphemeralSessionUsesACopyOfTheTPM(t *testing.T) {
	b, env, _ := uefiMachine(t)
	swtpmLog := fakeSwtpm(t, false)
	name := b.key(env)
	if err := os.MkdirAll(b.tpmStateDir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.tpmStateDir(name), "tpm2-00.permall"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.StartEphemeral(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(swtpmLog)
	if !strings.Contains(string(args), "dir="+b.tpmSessionDir(name)+",mode=0600") {
		t.Fatalf("swtpm should use the session copy: %s", args)
	}
	if data, err := os.ReadFile(filepath.Join(b.tpmSessionDir(name), "tpm2-00.permall")); err != nil || string(data) != "before" {
		t.Fatalf("session copy: %q %v", data, err)
	}

	// The fake QEMU never stays running, so Stop takes the "already
	// stopped" path, as it does after the guest shut itself down.
	if err := b.Stop(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.tpmSessionDir(name)); !os.IsNotExist(err) {
		t.Errorf("the session's TPM copy outlived it: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(b.tpmStateDir(name), "tpm2-00.permall")); string(data) != "before" {
		t.Errorf("the kept TPM state changed: %q", data)
	}
}

func TestStopEndsALeftoverSwtpm(t *testing.T) {
	b, env, _ := uefiMachine(t)
	fakeSwtpm(t, false)
	if err := b.Start(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(b.tpmPIDPath(b.key(env)))
	if err != nil {
		t.Fatal(err)
	}
	pid := strings.TrimSpace(string(data))
	if err := b.Stop(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat("/proc/" + pid); os.IsNotExist(err) {
			break
		}
		if stat, _ := os.ReadFile("/proc/" + pid + "/stat"); strings.Contains(string(stat), ") Z ") {
			break // exited, waiting to be reaped by this test process
		}
		if time.Now().After(deadline) {
			t.Fatalf("swtpm %s still running after Stop", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(b.tpmSocketPath(b.key(env))); !os.IsNotExist(err) {
		t.Errorf("stale TPM socket left: %v", err)
	}
}

// omavm host says what new Desktops get, and what to install otherwise.
func TestHostReportsUEFIAndTPM(t *testing.T) {
	if c := firmwareCapability(); c.Available || c.Hint != "Install edk2-ovmf" {
		t.Errorf("without OVMF: %+v", c)
	}
	if c := tpmCapability(); c.Available || c.Hint != "Install swtpm" {
		t.Errorf("without swtpm: %+v", c)
	}
	useFirmware(t, map[string]string{"60-plain.json": plainDescriptor})
	if c := firmwareCapability(); !c.Available || !strings.Contains(c.Detail, "without Secure Boot") {
		t.Errorf("plain OVMF: %+v", c)
	}
	useFirmware(t, map[string]string{"50-secure.json": secureDescriptor})
	if c := firmwareCapability(); !c.Available || !strings.Contains(c.Detail, "with Secure Boot") {
		t.Errorf("Secure Boot OVMF: %+v", c)
	}
	fakeSwtpm(t, false)
	if c := tpmCapability(); !c.Available {
		t.Errorf("with swtpm: %+v", c)
	}
}

// Force Stop of a running session that keeps no changes must not leave
// its copy of the TPM, keys and all, behind until the next Start.
func TestForceStopDropsTheSessionTPMCopy(t *testing.T) {
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "win", Kind: core.Machine}
	name := b.key(env)
	startFakeQEMU(t, b, name)
	if err := os.MkdirAll(b.tpmSessionDir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := b.ForceStop(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.tpmSessionDir(name)); !os.IsNotExist(err) {
		t.Errorf("the session's TPM copy outlived Force Stop: %v", err)
	}
}
