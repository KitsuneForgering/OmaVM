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

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// A UEFI Machine gets a TPM 2.0 from swtpm, one process per Machine like
// virtiofsd, with its state in the Machine's directory. Windows 11 refuses
// to install without one, and the state holds keys (BitLocker's, for
// one), so the directory is private to this user and goes with the
// Machine on Clone and Remove.

// swtpmLookPath finds swtpm; overridable in tests.
var swtpmLookPath = func() (string, error) { return exec.LookPath("swtpm") }

func (b *Backend) tpmStateDir(name string) string { return filepath.Join(b.dir(name), "tpm") }

// tpmSessionDir holds a throwaway copy of the TPM state for a session
// started without keeping changes, as -snapshot does for the disk and the
// UEFI variables.
func (b *Backend) tpmSessionDir(name string) string { return filepath.Join(b.dir(name), "tpm-session") }
func (b *Backend) tpmSocketPath(name string) string { return filepath.Join(b.dir(name), "swtpm.sock") }
func (b *Backend) tpmPIDPath(name string) string    { return filepath.Join(b.dir(name), "swtpm.pid") }

// tpmInUse reports whether the TPM has state worth protecting: swtpm
// writes it on the first boot that uses the TPM.
func (b *Backend) tpmInUse(name string) bool {
	entries, err := os.ReadDir(b.tpmStateDir(name))
	return err == nil && len(entries) > 0
}

// startTPM starts this Machine's swtpm and returns the QEMU arguments that
// connect to it, or none when the Machine boots with BIOS. Without swtpm
// the Machine still starts, unless its TPM already holds state: booting a
// system that sealed keys in it without the TPM would land it in BitLocker
// recovery, so that fails with what to install.
func (b *Backend) startTPM(ctx context.Context, name string, ephemeral bool) ([]string, error) {
	if _, uefi, err := b.machineFirmware(name); err != nil || !uefi {
		return nil, err
	}
	swtpm, err := swtpmLookPath()
	if err != nil {
		if b.tpmInUse(name) {
			return nil, fmt.Errorf("this Machine's TPM holds its keys, and swtpm is not installed (install it with: sudo pacman -S swtpm)")
		}
		slog.Warn("swtpm not found; starting without a TPM", "machine", name, "hint", "install swtpm")
		return nil, nil
	}
	b.stopTPM(name)
	state := b.tpmStateDir(name)
	if err := os.MkdirAll(state, 0o700); err != nil {
		return nil, fmt.Errorf("create TPM state: %w", err)
	}
	if ephemeral {
		session := b.tpmSessionDir(name)
		if out, err := exec.CommandContext(ctx, "cp", "-a", state, session).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("copy TPM state: %w: %s", err, strings.TrimSpace(string(out)))
		}
		state = session
	}
	args := []string{"socket", "--tpm2",
		"--tpmstate", "dir=" + state + ",mode=0600",
		"--ctrl", "type=unixio,path=" + b.tpmSocketPath(name),
		// Exits when QEMU closes the connection: a Machine that shuts
		// down from inside leaves no swtpm behind.
		"--terminate"}
	logPath := filepath.Join(b.dir(name), "swtpm.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("TPM: %w", err)
	}
	defer logFile.Close()
	// Outlives this command, like virtiofsd.
	cmd := exec.CommandContext(context.Background(), swtpm, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start swtpm: %w", err)
	}
	if err := os.WriteFile(b.tpmPIDPath(name), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("record swtpm pid: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.After(2 * time.Second)
	for {
		if _, err := os.Stat(b.tpmSocketPath(name)); err == nil {
			return []string{
				"-chardev", "socket,id=tpm,path=" + b.tpmSocketPath(name),
				"-tpmdev", "emulator,id=tpm0,chardev=tpm",
				"-device", "tpm-crb,tpmdev=tpm0",
			}, nil
		}
		select {
		case waitErr := <-exited:
			b.stopTPM(name)
			return nil, fmt.Errorf("the TPM could not start: %s", virtiofsdFailure(logPath, waitErr))
		case <-ctx.Done():
			b.stopTPM(name)
			return nil, ctx.Err()
		case <-deadline:
			b.stopTPM(name)
			return nil, errors.New("the TPM could not start: swtpm did not start in time")
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// stopTPM ends a swtpm QEMU never connected to or left behind, and drops a
// session's throwaway state. swtpm normally exits by itself with QEMU.
func (b *Backend) stopTPM(name string) {
	if data, err := os.ReadFile(b.tpmPIDPath(name)); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && processHasArg(pid, "type=unixio,path="+b.tpmSocketPath(name)) {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Signal(syscall.SIGTERM)
			}
		}
	}
	_ = os.Remove(b.tpmPIDPath(name))
	_ = os.Remove(b.tpmSocketPath(name))
	_ = os.RemoveAll(b.tpmSessionDir(name))
}

func tpmCapability() core.HostCapability {
	c := core.HostCapability{ID: "tpm", Label: "TPM for Desktops"}
	if _, err := swtpmLookPath(); err != nil {
		c.Detail = "swtpm not found, so Desktops start without a TPM and can't install Windows 11"
		c.Hint = "Install swtpm"
		return c
	}
	c.Available = true
	c.Detail = "a TPM 2.0 for each Desktop with UEFI, its keys kept in that Desktop's own folder"
	return c
}
