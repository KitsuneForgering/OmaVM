package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

func testLauncher(t *testing.T) *Launcher {
	t.Helper()
	return &Launcher{Dir: filepath.Join(t.TempDir(), "applications"), CLI: "/opt/omavm/omavm", GUI: "/opt/omavm/omavm-gui"}
}

func readEntry(t *testing.T, l *Launcher, env core.Environment) string {
	t.Helper()
	data, err := os.ReadFile(l.path(env))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMachineEntryOpensThroughCLI(t *testing.T) {
	l := testLauncher(t)
	env := core.Environment{ID: "abc", Name: "Fedora", Kind: core.Machine}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	entry := readEntry(t, l, env)
	for _, want := range []string{
		"Name=Fedora\n",
		`Exec="env" "OMAVM_NOTIFY_ERRORS=1" "/opt/omavm/omavm" "open" "Fedora"` + "\n",
		"Terminal=false\n",
		"Icon=dev.omavm.app\n",
	} {
		if !strings.Contains(entry, want) {
			t.Errorf("entry missing %q:\n%s", want, entry)
		}
	}
}

func TestBoxEntryOpensEmbeddedTerminal(t *testing.T) {
	l := testLauncher(t)
	env := core.Environment{ID: "b1", Name: "dev box", Kind: core.Box}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	want := `Exec="/opt/omavm/omavm-gui" "--terminal" "dev box" "--title" "dev box — OmaVM"` + "\n"
	if entry := readEntry(t, l, env); !strings.Contains(entry, want) {
		t.Errorf("entry missing %q:\n%s", want, entry)
	}
}

func TestBoxEntryWithoutGUIUsesTerminal(t *testing.T) {
	l := testLauncher(t)
	l.GUI = ""
	env := core.Environment{ID: "b1", Name: "dev", Kind: core.Box}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	entry := readEntry(t, l, env)
	if !strings.Contains(entry, "Terminal=true\n") || !strings.Contains(entry, `"open" "dev"`) {
		t.Errorf("expected a terminal entry running omavm open:\n%s", entry)
	}
}

// Names may contain anything but '/', '\' and control characters, so the
// Exec quoting has to hold up against shell and field-code characters.
func TestExecQuotesSpecialCharacters(t *testing.T) {
	got := execLine([]string{"omavm", `a"b$c` + "`d%e"})
	want := `"omavm" "a\"b\$c` + "\\`" + `d%%e"`
	if got != want {
		t.Fatalf("execLine = %s, want %s", got, want)
	}
	// The value escaping doubles the backslashes execLine added.
	if escaped := escapeValue(`"a\"b"`); escaped != `"a\\"b"` {
		t.Fatalf("escapeValue = %s", escaped)
	}
}

func TestPublishSkipsUnchangedEntry(t *testing.T) {
	l := testLauncher(t)
	env := core.Environment{ID: "abc", Name: "Fedora", Kind: core.Machine}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(l.path(env), old, old); err != nil {
		t.Fatal(err)
	}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(l.path(env))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Fatal("unchanged entry was rewritten")
	}
}

func TestWithdrawRemovesEntryAndToleratesMissing(t *testing.T) {
	l := testLauncher(t)
	env := core.Environment{ID: "abc", Name: "Fedora", Kind: core.Machine}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := l.Withdraw(env); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(l.path(env)); !os.IsNotExist(err) {
		t.Fatalf("entry still exists: %v", err)
	}
}

func TestColorIconFromDataDir(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	icon := filepath.Join(data, "omavm", "icons", "green.svg")
	if err := os.MkdirAll(filepath.Dir(icon), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(icon, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ColorIcon("green"); got != icon {
		t.Fatalf("ColorIcon = %q, want %q", got, icon)
	}
	if got := ColorIcon(""); got != "" {
		t.Fatalf("no color should have no icon, got %q", got)
	}

	l := testLauncher(t)
	env := core.Environment{ID: "abc", Name: "Fedora", Kind: core.Machine, Settings: core.EnvironmentSettings{Color: "green"}}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	if entry := readEntry(t, l, env); !strings.Contains(entry, "Icon="+icon+"\n") {
		t.Errorf("entry should use the color icon:\n%s", entry)
	}
}

// Regression: a Box opened from its launcher entry ignored "open in an
// empty workspace" being turned off, because only the Experience Center
// knew the setting.
func TestBoxEntryCarriesTheWorkspaceSetting(t *testing.T) {
	l := testLauncher(t)
	env := core.Environment{ID: "abc", Name: "dev", Kind: core.Box, Settings: core.EnvironmentSettings{EmptyWorkspaceDisabled: true}}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	if entry := readEntry(t, l, env); !strings.Contains(entry, `"--empty-workspace" "false"`) {
		t.Fatalf("setting not passed to the terminal:\n%s", entry)
	}
}

// A Box opened from the launcher shows its color in the terminal, like one
// opened from the Experience Center.
func TestBoxEntryCarriesTheColor(t *testing.T) {
	l := testLauncher(t)
	env := core.Environment{ID: "abc", Name: "dev", Kind: core.Box, Settings: core.EnvironmentSettings{Color: "green"}}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	if entry := readEntry(t, l, env); !strings.Contains(entry, `"--color" "green"`) {
		t.Fatalf("color not passed to the terminal:\n%s", entry)
	}
	env.Settings.Color = ""
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	if entry := readEntry(t, l, env); strings.Contains(entry, "--color") {
		t.Fatalf("a Box without a color got one:\n%s", entry)
	}
}

// Regression: installed from the release tarball (OmaStore), OmaVM isn't
// in the icon theme, so entries named a themed icon nothing provides.
func TestEntryUsesTheBundledIconOutsideTheIconTheme(t *testing.T) {
	root := t.TempDir()
	svg := filepath.Join(root, "data", "icons", "dev.omavm.app.svg")
	if err := os.MkdirAll(filepath.Dir(svg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svg, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := &Launcher{Dir: filepath.Join(t.TempDir(), "applications"), CLI: filepath.Join(root, "bin", "omavm")}
	env := core.Environment{ID: "abc", Name: "vm", Kind: core.Machine}
	if err := l.Publish(env); err != nil {
		t.Fatal(err)
	}
	if entry := readEntry(t, l, env); !strings.Contains(entry, "Icon="+svg+"\n") {
		t.Fatalf("entry doesn't use the bundled icon:\n%s", entry)
	}
}

// End to end through a real launcher (GLib's, which GNOME and most
// launchers use): whatever the environment is called, its entry runs
// `omavm open NAME` with the name intact. Names from 2026-10-07's fuzzing
// of execLine, each a way out of naive quoting.
func TestEntryLaunchesTheEnvironmentThroughGLib(t *testing.T) {
	gio, err := exec.LookPath("gio")
	if err != nil {
		t.Skip("needs gio")
	}
	dir := t.TempDir()
	argv := filepath.Join(dir, "argv")
	cli := filepath.Join(dir, "omavm")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '[%s]\\n' \"$a\"; done > " + argv + "\n"
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	l := &Launcher{Dir: filepath.Join(dir, "applications"), CLI: cli}
	for i, name := range []string{`a"b`, "c`d", "$HOME", `back\slash`, "50% off", "%u", " lead", `\"`, "it's", "a&b|c;d", "Ação 🙂"} {
		env := core.Environment{ID: fmt.Sprintf("e%d", i), Name: name, Kind: core.Machine}
		if err := l.Publish(env); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(argv)
		if out, err := exec.Command(gio, "launch", l.path(env)).CombinedOutput(); err != nil {
			// A host where gio can't launch anything (no session, as on
			// some CI runners) says nothing about the entries.
			if i == 0 {
				t.Skipf("gio launch doesn't work here: %v: %s", err, out)
			}
			t.Fatalf("gio launch: %v: %s", err, out)
		}
		var got []byte
		for range 100 {
			if got, err = os.ReadFile(argv); err == nil && len(got) > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if want := "[open]\n[" + name + "]\n"; string(got) != want {
			t.Errorf("name %q: the entry ran %q, want %q", name, got, want)
		}
	}
}
