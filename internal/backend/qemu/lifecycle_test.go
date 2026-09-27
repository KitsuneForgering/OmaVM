package qemu

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

func TestSlowShutdownNeverSendsQuit(t *testing.T) {
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "guest", Kind: core.Machine}
	if err := os.MkdirAll(b.dir(env.Name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.pidPath(env.Name), []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", b.qmpPath(env.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	commands := make(chan string, 10)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			enc, dec := json.NewEncoder(conn), json.NewDecoder(conn)
			enc.Encode(map[string]any{"QMP": map[string]any{}})
			var req struct {
				Execute string `json:"execute"`
			}
			if dec.Decode(&req) == nil {
				enc.Encode(map[string]any{"return": map[string]any{}})
				if dec.Decode(&req) == nil {
					commands <- req.Execute
					enc.Encode(map[string]any{"return": map[string]any{"status": "running"}})
				}
			}
			conn.Close()
		}
	}()
	err = b.Stop(context.Background(), env)
	listener.Close()
	<-done
	close(commands)
	if err == nil || !strings.Contains(err.Error(), "left running") {
		t.Fatalf("slow shutdown: %v", err)
	}
	for command := range commands {
		if command != "query-status" && command != "system_powerdown" {
			t.Errorf("unsafe shutdown command: %s", command)
		}
	}
}

func TestShutdownFailurePreservesGuest(t *testing.T) {
	for _, action := range []string{"stop", "restart", "remove"} {
		t.Run(action, func(t *testing.T) {
			child := exec.Command("sleep", "60")
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { child.Process.Kill(); child.Wait() }()
			b := &Backend{stateDir: t.TempDir()}
			env := core.Environment{Name: "guest", Kind: core.Machine}
			if err := os.MkdirAll(b.dir(env.Name), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(b.pidPath(env.Name), []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch action {
			case "stop":
				err = b.Stop(context.Background(), env)
			case "restart":
				err = b.Restart(context.Background(), env)
			case "remove":
				err = b.Remove(context.Background(), env)
			}
			if err == nil {
				t.Fatal("expected shutdown failure")
			}
			if err := child.Process.Signal(syscall.Signal(0)); err != nil {
				t.Fatalf("guest was killed: %v", err)
			}
			if _, err := os.Stat(b.dir(env.Name)); err != nil {
				t.Fatalf("guest data removed: %v", err)
			}
		})
	}
}

func TestGoToSnapshotPausesAroundRunningRestore(t *testing.T) {
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "guest", Kind: core.Machine}
	if err := os.MkdirAll(b.dir(env.Name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.pidPath(env.Name), []byte(strconv.Itoa(child.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", b.qmpPath(env.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var commands []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			enc, dec := json.NewEncoder(conn), json.NewDecoder(conn)
			enc.Encode(map[string]any{"QMP": map[string]any{}})
			var req struct {
				Execute string `json:"execute"`
			}
			if dec.Decode(&req) == nil { // qmp_capabilities
				enc.Encode(map[string]any{"return": map[string]any{}})
				if dec.Decode(&req) == nil {
					commands = append(commands, req.Execute)
					if req.Execute == "human-monitor-command" {
						enc.Encode(map[string]any{"return": ""})
					} else {
						enc.Encode(map[string]any{"return": map[string]any{}})
					}
				}
			}
			conn.Close()
		}
	}()
	restoreErr := b.GoToSnapshot(context.Background(), env, "before-upgrade")
	listener.Close()
	<-done
	if restoreErr != nil {
		t.Fatalf("GoToSnapshot: %v", restoreErr)
	}
	want := []string{"stop", "human-monitor-command", "cont"}
	if len(commands) != len(want) {
		t.Fatalf("expected commands %v, got %v", want, commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Fatalf("expected commands %v, got %v", want, commands)
		}
	}
}

func TestSnapshotLifecycleOnStoppedMachine(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available")
	}
	b := &Backend{stateDir: t.TempDir()}
	env := core.Environment{Name: "guest", Kind: core.Machine}
	if err := b.Create(context.Background(), env); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ctx := context.Background()
	if err := b.CreateSnapshot(ctx, env, "clean-install"); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	out, err := runOutput(ctx, "qemu-img", "snapshot", "-l", b.diskPath(env.Name))
	if err != nil {
		t.Fatalf("qemu-img snapshot -l: %v: %s", err, out)
	}
	if !strings.Contains(out, "clean-install") {
		t.Fatalf("expected snapshot to be listed, got: %s", out)
	}

	if err := b.GoToSnapshot(ctx, env, "clean-install"); err != nil {
		t.Fatalf("GoToSnapshot: %v", err)
	}
	if err := b.RemoveSnapshot(ctx, env, "clean-install"); err != nil {
		t.Fatalf("RemoveSnapshot: %v", err)
	}
	out, err = runOutput(ctx, "qemu-img", "snapshot", "-l", b.diskPath(env.Name))
	if err != nil {
		t.Fatalf("qemu-img snapshot -l: %v: %s", err, out)
	}
	if strings.Contains(out, "clean-install") {
		t.Fatalf("expected snapshot to be removed, got: %s", out)
	}
}

func TestTravelModeReducesCPUsOnBattery(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	t.Setenv("OMAVM_TEST_ARGS", capture)
	t.Setenv("PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, "qemu-system-x86_64"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OMAVM_TEST_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	orig := powerSupplyDir
	defer func() { powerSupplyDir = orig }()
	powerSupplyDir = filepath.Join(dir, "power_supply")
	writePowerSupply(t, powerSupplyDir, "AC", "Mains", "0")

	b := &Backend{stateDir: dir}
	env := core.Environment{Name: "guest", Kind: core.Machine}
	if err := b.Start(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "-smp\n1\n") {
		t.Fatalf("expected halved default CPUs (1) on battery, got: %s", data)
	}

	// A user-pinned CPU count must never be overridden by Travel Mode.
	if err := os.Remove(b.pidPath(env.Name)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	env.Settings.CPUs = 4
	if err := b.Start(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "-smp\n4\n") {
		t.Fatalf("expected pinned CPU count to survive Travel Mode, got: %s", data)
	}

	// Opt-out: TravelModeDisabled must stop the reduction even on
	// battery with default CPUs.
	if err := os.Remove(b.pidPath(env.Name)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	env.Settings.CPUs = 0
	env.Settings.TravelModeDisabled = true
	if err := b.Start(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "-smp\n2\n") {
		t.Fatalf("expected Travel Mode opt-out to keep default CPUs (2), got: %s", data)
	}
}

func TestBootMediaArguments(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	t.Setenv("OMAVM_TEST_ARGS", capture)
	t.Setenv("PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, "qemu-system-x86_64"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OMAVM_TEST_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	b := &Backend{stateDir: dir}
	env := core.Environment{Name: "guest", Kind: core.Machine, Image: "/missing/installer.iso"}
	for _, disconnect := range []bool{false, true} {
		env.Settings.DisconnectISO = disconnect
		if err := b.Start(context.Background(), env); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal(err)
		}
		args := string(data)
		if strings.Contains(args, "-cdrom\n") == disconnect {
			t.Fatalf("unexpected media arguments: %s", args)
		}
		if !disconnect && !strings.Contains(args, "order=cd,menu=on") {
			t.Fatalf("disk must precede installer: %s", args)
		}
	}
}
