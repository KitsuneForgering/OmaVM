package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The GUI and the Omarchy bar run omavm every few seconds; without a cap
// the log grew without end (1.5 MB in two days on the development host).
func TestOpenRotatesALogPastTheLimit(t *testing.T) {
	if writable(SystemLogDir) {
		t.Skip("logs go to " + SystemLogDir + " on this host")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	old := maxLogSize
	maxLogSize = 100
	t.Cleanup(func() { maxLogSize = old })

	dir, err := logDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "omavm.log")
	big := strings.Repeat("x", 200)
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}

	_, closeLog, err := Open("omavm")
	if err != nil {
		t.Fatal(err)
	}
	closeLog()
	rotated, err := os.ReadFile(path + ".1")
	if err != nil || string(rotated) != big {
		t.Fatalf("previous log not kept as .1: %q, %v", rotated, err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() >= int64(len(big)) {
		t.Fatalf("log was not restarted after rotation: %v, %v", info, err)
	}
}

// "log opened" says where the log lives; once per file is enough, not a
// line on every one of the thousands of invocations.
func TestLogOpenedOnlyForANewFile(t *testing.T) {
	if writable(SystemLogDir) {
		t.Skip("logs go to " + SystemLogDir + " on this host")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for i := 0; i < 3; i++ {
		_, closeLog, err := Open("omavm")
		if err != nil {
			t.Fatal(err)
		}
		closeLog()
	}
	dir, _ := logDir()
	data, err := os.ReadFile(filepath.Join(dir, "omavm.log"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "log opened"); n != 1 {
		t.Fatalf("\"log opened\" written %d times, want once", n)
	}
}
