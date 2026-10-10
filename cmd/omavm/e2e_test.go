package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// cliEnv runs the real omavm binary as a separate process against its own
// state, the way the GUI, the bar and agents run it: exit codes, stdout
// JSON and stderr are the contract, not Go return values.
type cliEnv struct {
	t   *testing.T
	bin string
	env []string
}

func newCLIEnv(t *testing.T) *cliEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the omavm binary")
	}
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("needs qemu-img")
	}
	// Short on purpose: a Machine's sockets live under XDG_STATE_HOME and
	// must fit in 108 bytes, which a test's own TempDir can exceed.
	dir, err := os.MkdirTemp("", "ovm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	bin := filepath.Join(dir, "omavm")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(),
		"HOME="+home,
		"XDG_STATE_HOME="+filepath.Join(dir, "s"),
		"XDG_DATA_HOME="+filepath.Join(dir, "data"),
		"XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
		// No container engine: only Machines here, and no Box backend probing.
		"DBX_CONTAINER_MANAGER=",
	)
	return &cliEnv{t: t, bin: bin, env: env}
}

// run returns stdout, stderr and the exit code.
func (c *cliEnv) run(args ...string) (string, string, int) {
	c.t.Helper()
	cmd := exec.Command(c.bin, args...)
	cmd.Env = c.env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		c.t.Fatalf("omavm %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), code
}

func (c *cliEnv) ok(args ...string) string {
	c.t.Helper()
	out, errOut, code := c.run(args...)
	if code != 0 {
		c.t.Fatalf("omavm %s: exit %d: %s", strings.Join(args, " "), code, errOut)
	}
	return out
}

func (c *cliEnv) json(v any, args ...string) {
	c.t.Helper()
	if err := json.Unmarshal([]byte(c.ok(args...)), v); err != nil {
		c.t.Fatalf("omavm %s: %v", strings.Join(args, " "), err)
	}
}

// A Machine's whole life that needs no boot, through the binary: create,
// list, settings, a snapshot of the stopped disk, a clone, removal. Uses
// the real qemu-img; no QEMU is started.
func TestCLIMachineLifecycleEndToEnd(t *testing.T) {
	c := newCLIEnv(t)
	iso := filepath.Join(t.TempDir(), "system.iso")
	if err := os.WriteFile(iso, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	name := "Windows 11, trabalho" // a comma, which QEMU's options split on
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	c.json(&created, "create", "--name", name, "--kind", "machine", "--image", iso, "--json")
	if created.ID == "" || created.Name != name {
		t.Fatalf("create --json: %+v", created)
	}

	var list []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Status struct {
			State string `json:"state"`
		} `json:"status"`
	}
	c.json(&list, "list", "--status", "--json")
	if len(list) != 1 || list[0].ID != created.ID || list[0].Name != name || list[0].Kind != "machine" || list[0].Status.State != "stopped" {
		t.Fatalf("list after create: %+v", list)
	}

	var settings struct {
		Color string `json:"color"`
	}
	c.ok("settings", name, "--color", "blue")
	c.json(&settings, "settings", name, "--json")
	if settings.Color != "blue" {
		t.Fatalf("color not saved: %+v", settings)
	}

	c.ok("snapshot", "create", name, "--label", "Clean installation")
	var snapshots []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	c.json(&snapshots, "snapshot", "list", name, "--json")
	if len(snapshots) != 1 || snapshots[0].Label != "Clean installation" {
		t.Fatalf("snapshots: %+v", snapshots)
	}
	c.ok("snapshot", "go-to", name, snapshots[0].ID)

	c.ok("clone", name, "copy")
	c.json(&list, "list", "--json")
	if len(list) != 2 {
		t.Fatalf("list after clone: %+v", list)
	}
	c.json(&snapshots, "snapshot", "list", "copy", "--json")
	if len(snapshots) != 1 {
		t.Fatalf("the clone lost its snapshot: %+v", snapshots)
	}

	c.ok("remove", "copy")
	c.ok("remove", name)
	c.json(&list, "list", "--json")
	if len(list) != 0 {
		t.Fatalf("list after removing both: %+v", list)
	}
}

// Exit codes and JSON errors are what scripts and agents branch on
// (cmd/omavm/errors.go): checked on the process, where they are produced.
func TestCLIExitCodesEndToEnd(t *testing.T) {
	c := newCLIEnv(t)
	iso := filepath.Join(t.TempDir(), "system.iso")
	if err := os.WriteFile(iso, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	c.ok("create", "--name", "vm", "--kind", "machine", "--image", iso)
	for _, tc := range []struct {
		args     []string
		code     int
		jsonCode string
	}{
		{[]string{"status", "nothing", "--json"}, 3, "not_found"},
		{[]string{"create", "--name", "vm", "--kind", "machine", "--image", iso, "--json"}, 4, "already_exists"},
		// Not UTF-8: it would be stored as something else and never found.
		{[]string{"create", "--name", "bad\xffname", "--kind", "machine", "--image", iso, "--json"}, 2, "invalid_input"},
		{[]string{"create", "--name", "-dash", "--kind", "machine", "--image", iso, "--json"}, 2, "invalid_input"},
		{[]string{"create", "--name", "x", "--kind", "machine", "--image", "/no/such.iso", "--json"}, 2, "invalid_input"},
		{[]string{"status", "--json"}, 2, ""},
	} {
		out, _, code := c.run(tc.args...)
		if code != tc.code {
			t.Errorf("omavm %q: exit %d, want %d (%s)", tc.args, code, tc.code, out)
			continue
		}
		if tc.jsonCode == "" {
			continue
		}
		var reply struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(out), &reply); err != nil || reply.Error.Message == "" {
			t.Errorf("omavm %q: no JSON error on stdout: %q", tc.args, out)
		} else if reply.Error.Code != tc.jsonCode {
			t.Errorf("omavm %q: error code %q, want %q", tc.args, reply.Error.Code, tc.jsonCode)
		}
	}
	// The failed creations left nothing behind.
	var list []json.RawMessage
	c.json(&list, "list", "--json")
	if len(list) != 1 {
		t.Fatalf("failed creations changed the registry: %d environments", len(list))
	}
}

// A Box's real life through Distrobox and Podman: create, the first exec
// (which runs Distrobox's setup), exit codes, stop, removal leaving no
// container behind. Opt-in (OMAVM_E2E_BOX=1): it creates real containers
// and takes ~30 s. Uses alpine from local storage, never the network.
func TestCLIBoxLifecycleEndToEnd(t *testing.T) {
	if os.Getenv("OMAVM_E2E_BOX") != "1" {
		t.Skip("set OMAVM_E2E_BOX=1 to run a real Box through Distrobox")
	}
	const image = "docker.io/library/alpine:latest"
	for _, tool := range []string{"distrobox", "podman"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("needs %s", tool)
		}
	}
	if exec.Command("podman", "image", "exists", image).Run() != nil {
		t.Skipf("needs %s in local storage (podman pull %s)", image, image)
	}
	// Podman keeps images under XDG_DATA_HOME, which newCLIEnv isolates:
	// point it back at this user's store, where the image already is.
	store, err := exec.Command("podman", "info", "--format", "{{.Store.GraphDriverName}}\n{{.Store.GraphRoot}}\n{{.Store.RunRoot}}").Output()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(strings.TrimSpace(string(store)), "\n")
	if len(fields) != 3 {
		t.Fatalf("podman info: %q", store)
	}
	conf := filepath.Join(t.TempDir(), "storage.conf")
	if err := os.WriteFile(conf, []byte(fmt.Sprintf("[storage]\ndriver = %q\ngraphroot = %q\nrunroot = %q\n", fields[0], fields[1], fields[2])), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newCLIEnv(t)
	c.env = append(c.env, "CONTAINERS_STORAGE_CONF="+conf)
	var created struct {
		ID string `json:"id"`
	}
	c.json(&created, "create", "--name", "e2e box", "--image", image, "--json")
	t.Cleanup(func() { c.run("remove", "e2e box") })

	// Distrobox's setup progress goes to stderr: stdout is the command's.
	out := c.ok("exec", "e2e box", "--", "cat", "/etc/alpine-release")
	if strings.TrimSpace(out) == "" || strings.ContainsAny(out, "\x1b[") {
		t.Fatalf("exec stdout carries more than the command's output: %q", out)
	}
	// The command failing is its exit code alone, with no words from omavm.
	_, errOut, code := c.run("exec", "e2e box", "--", "false")
	if code != 1 || strings.Contains(errOut, "omavm:") {
		t.Fatalf("exec of false: exit %d, stderr %q", code, errOut)
	}

	c.ok("stop", "e2e box")
	var status struct {
		State string `json:"state"`
	}
	c.json(&status, "status", "e2e box", "--json")
	if status.State != "stopped" {
		t.Fatalf("after stop: %q", status.State)
	}
	c.ok("remove", "e2e box")
	names, _ := exec.Command("podman", "ps", "-a", "--format", "{{.Names}}").Output()
	if strings.Contains(string(names), created.ID[:8]) {
		t.Fatalf("removing the Box left its container: %s", names)
	}
}

// A Machine on a real QEMU, no system needed: its firmware runs with
// nothing to boot. Covers what only fakes did: a snapshot of the running
// disk over QMP (NVMe's drive, disk0), Go To refused while it runs, Force
// Stop, a session that keeps no changes. Opt-in (OMAVM_E2E_QEMU=1).
func TestCLIMachineOnRealQEMUEndToEnd(t *testing.T) {
	if os.Getenv("OMAVM_E2E_QEMU") != "1" {
		t.Skip("set OMAVM_E2E_QEMU=1 to run a Machine on a real QEMU")
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("needs /dev/kvm")
	}
	if _, err := exec.LookPath("qemu-system-x86_64"); err != nil {
		t.Skip("needs qemu-system-x86_64")
	}
	c := newCLIEnv(t)
	iso := filepath.Join(t.TempDir(), "nothing.iso")
	if err := os.WriteFile(iso, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	c.ok("create", "--name", "vm", "--kind", "machine", "--image", iso, "--memory-mib", "512")
	t.Cleanup(func() {
		c.run("force-stop", "vm")
		c.run("remove", "vm")
	})
	status := func() (state string, ephemeral bool) {
		var s struct {
			State     string `json:"state"`
			Ephemeral bool   `json:"ephemeral"`
		}
		c.json(&s, "status", "vm", "--json")
		return s.State, s.Ephemeral
	}

	c.ok("start", "vm")
	if state, _ := status(); state != "running" {
		t.Fatalf("after start: %q", state)
	}
	c.ok("snapshot", "create", "vm", "--label", "while running")
	var snapshots []struct {
		ID              string `json:"id"`
		Label           string `json:"label"`
		CrashConsistent bool   `json:"crash_consistent"`
	}
	c.json(&snapshots, "snapshot", "list", "vm", "--json")
	if len(snapshots) != 1 || snapshots[0].Label != "while running" || !snapshots[0].CrashConsistent {
		t.Fatalf("snapshot of the running Machine (no guest agent, so crash-consistent): %+v", snapshots)
	}
	// QEMU only reverts a disk snapshot offline.
	if _, _, code := c.run("snapshot", "go-to", "vm", snapshots[0].ID); code != 2 {
		t.Fatalf("go-to on a running Machine: exit %d, want 2", code)
	}

	c.ok("force-stop", "vm")
	if state, _ := status(); state != "stopped" {
		t.Fatalf("after force-stop: %q", state)
	}
	c.ok("snapshot", "go-to", "vm", snapshots[0].ID)

	c.ok("start", "vm", "--ephemeral")
	if state, ephemeral := status(); state != "running" || !ephemeral {
		t.Fatalf("ephemeral session: %q, ephemeral %v", state, ephemeral)
	}
	if _, _, code := c.run("snapshot", "create", "vm", "--label", "lost"); code == 0 {
		t.Fatal("a snapshot during a session that keeps no changes was accepted")
	}
	c.ok("force-stop", "vm")
	c.ok("remove", "vm")
	if out, _ := exec.Command("pgrep", "-f", c.bin+"|"+t.TempDir()).Output(); len(out) > 0 {
		t.Logf("processes still mentioning the test: %s", out)
	}
}

// Processes racing on the registry: the GUI, the bar, launcher entries
// and agents run omavm at the same time. Each change must land, and a
// name can only be taken once.
func TestCLIConcurrentProcessesEndToEnd(t *testing.T) {
	c := newCLIEnv(t)
	iso := filepath.Join(t.TempDir(), "system.iso")
	if err := os.WriteFile(iso, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	parallel := func(n int, args func(i int) []string) []int {
		codes := make([]int, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, codes[i] = c.run(args(i)...)
			}()
		}
		wg.Wait()
		return codes
	}

	// Different names: all land.
	for i, code := range parallel(8, func(i int) []string {
		return []string{"create", "--name", fmt.Sprintf("vm%d", i), "--kind", "machine", "--image", iso}
	}) {
		if code != 0 {
			t.Fatalf("create vm%d: exit %d", i, code)
		}
	}
	var list []struct {
		Name     string `json:"name"`
		Settings struct {
			Color string `json:"color"`
		} `json:"settings"`
	}
	c.json(&list, "list", "--json")
	if len(list) != 8 {
		t.Fatalf("8 concurrent creates left %d environments", len(list))
	}

	// The same name: exactly one wins, the others are told it's taken.
	won, taken := 0, 0
	for _, code := range parallel(6, func(int) []string {
		return []string{"create", "--name", "same", "--kind", "machine", "--image", iso}
	}) {
		switch code {
		case 0:
			won++
		case 4:
			taken++
		default:
			t.Errorf("create of a contested name: exit %d", code)
		}
	}
	if won != 1 || taken != 5 {
		t.Fatalf("contested name: %d won, %d refused; want 1 and 5", won, taken)
	}

	// Settings of different environments at once: none is lost.
	colors := []string{"red", "orange", "yellow", "green", "blue", "purple", "gray", "red"}
	for i, code := range parallel(8, func(i int) []string {
		return []string{"settings", fmt.Sprintf("vm%d", i), "--color", colors[i]}
	}) {
		if code != 0 {
			t.Fatalf("settings vm%d: exit %d", i, code)
		}
	}
	c.json(&list, "list", "--json")
	got := map[string]string{}
	for _, env := range list {
		got[env.Name] = env.Settings.Color
	}
	for i, color := range colors {
		if got[fmt.Sprintf("vm%d", i)] != color {
			t.Errorf("vm%d: color %q, want %q (a concurrent save lost it)", i, got[fmt.Sprintf("vm%d", i)], color)
		}
	}
}

// A damaged registry stops everything with a way out, and the way out
// works: the previous version kept next to it.
func TestCLIDamagedRegistryRecoversFromBackupEndToEnd(t *testing.T) {
	c := newCLIEnv(t)
	iso := filepath.Join(t.TempDir(), "system.iso")
	if err := os.WriteFile(iso, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	c.ok("create", "--name", "keep me", "--kind", "machine", "--image", iso)
	c.ok("settings", "keep me", "--color", "green") // a second save: .bak holds the first
	var state string
	for _, kv := range c.env {
		if v, ok := strings.CutPrefix(kv, "XDG_STATE_HOME="); ok {
			state = v
		}
	}
	registry := filepath.Join(state, "omavm", "environments.json")
	if err := os.WriteFile(registry, []byte(`{"environments": [{"name": `), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errOut, code := c.run("list", "--json")
	if code == 0 {
		t.Fatalf("a damaged registry listed: %s", out)
	}
	if !strings.Contains(errOut, registry+".bak") {
		t.Fatalf("the error doesn't say where the backup is: %s", errOut)
	}
	// Nothing may write over the damaged file meanwhile.
	if _, _, code := c.run("create", "--name", "new", "--kind", "machine", "--image", iso); code == 0 {
		t.Fatal("create succeeded on a damaged registry")
	}
	backup, err := os.ReadFile(registry + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registry, backup, 0o644); err != nil {
		t.Fatal(err)
	}
	var list []struct {
		Name string `json:"name"`
	}
	c.json(&list, "list", "--json")
	if len(list) != 1 || list[0].Name != "keep me" {
		t.Fatalf("after restoring the backup: %+v", list)
	}
}

// Two Starts of one Machine at once (two clicks, a click and a launcher
// entry) end with it running once; reads and a snapshot alongside wait
// their turn instead of failing. Opt-in (OMAVM_E2E_QEMU=1).
func TestCLIConcurrentStartsOnRealQEMUEndToEnd(t *testing.T) {
	if os.Getenv("OMAVM_E2E_QEMU") != "1" {
		t.Skip("set OMAVM_E2E_QEMU=1 to run a Machine on a real QEMU")
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("needs /dev/kvm")
	}
	c := newCLIEnv(t)
	iso := filepath.Join(t.TempDir(), "nothing.iso")
	if err := os.WriteFile(iso, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID string `json:"id"`
	}
	c.json(&created, "create", "--name", "vm", "--kind", "machine", "--image", iso, "--memory-mib", "512", "--json")
	t.Cleanup(func() {
		c.run("force-stop", "vm")
		c.run("remove", "vm")
	})

	run := func(args ...string) <-chan int {
		done := make(chan int, 1)
		go func() {
			_, errOut, code := c.run(args...)
			if code != 0 {
				t.Logf("omavm %v: exit %d: %s", args, code, errOut)
			}
			done <- code
		}()
		return done
	}
	a, b := run("start", "vm"), run("start", "vm")
	if codeA, codeB := <-a, <-b; codeA != 0 || codeB != 0 {
		t.Fatalf("concurrent starts: exits %d and %d, want both 0", codeA, codeB)
	}
	qemus, _ := exec.Command("pgrep", "-f", "qemu-system-x86_64 .*"+created.ID).Output()
	if n := len(strings.Fields(string(qemus))); n != 1 {
		t.Fatalf("%d QEMU processes for one Machine, want 1", n)
	}

	snap, status := run("snapshot", "create", "vm", "--label", "concurrent"), run("status", "vm", "--json")
	if <-snap != 0 || <-status != 0 {
		t.Fatal("a snapshot and a status read at the same time failed")
	}
}

// omavm run --ephemeral: a Machine that exists while the command runs.
// Ctrl+C turns it off and deletes it: nothing in the registry, no QEMU,
// no state directory left. Opt-in (OMAVM_E2E_QEMU=1).
func TestCLIEphemeralRunCleansUpOnInterruptEndToEnd(t *testing.T) {
	if os.Getenv("OMAVM_E2E_QEMU") != "1" {
		t.Skip("set OMAVM_E2E_QEMU=1 to run a Machine on a real QEMU")
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("needs /dev/kvm")
	}
	c := newCLIEnv(t)
	iso := filepath.Join(t.TempDir(), "nothing.iso")
	if err := os.WriteFile(iso, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(c.bin, "run", "--ephemeral", "--name", "scratch", "--image", iso, "--memory-mib", "512", "--no-open")
	cmd.Env = c.env
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		c.run("force-stop", "scratch")
		c.run("remove", "scratch")
	})
	var id string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var s []struct {
			ID     string `json:"id"`
			Status struct {
				State string `json:"state"`
			} `json:"status"`
		}
		c.json(&s, "list", "--status", "--json")
		if len(s) == 1 && s[0].Status.State == "running" {
			id = s[0].ID
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if id == "" {
		t.Fatalf("the ephemeral Machine never ran: %s", out.String())
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case <-waited:
	case <-time.After(30 * time.Second):
		t.Fatalf("run didn't end after Ctrl+C: %s", out.String())
	}
	var list []json.RawMessage
	c.json(&list, "list", "--json")
	if len(list) != 0 {
		t.Fatalf("Ctrl+C left %d environments: %s", len(list), out.String())
	}
	if pids, _ := exec.Command("pgrep", "-f", "qemu-system-x86_64 .*"+id).Output(); len(pids) > 0 {
		t.Fatalf("Ctrl+C left QEMU running: %s", pids)
	}
	var state string
	for _, kv := range c.env {
		if v, ok := strings.CutPrefix(kv, "XDG_STATE_HOME="); ok {
			state = v
		}
	}
	if _, err := os.Stat(filepath.Join(state, "omavm", "machines", id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Ctrl+C left the Machine's state directory: %v", err)
	}
}
