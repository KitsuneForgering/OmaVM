// Package applog opens the structured logger shared by the omavm CLI
// and the omavm-gui Experience Center.
//
// /var/log is root-owned (0755 root:root) on a stock Linux install, and
// CLAUDE.md's Security Model forbids silently elevating privileges to
// work around that. So Open prefers /var/log/omavm when it already
// exists and is writable — the standard location an admin or installer
// can provision once (see README's "Logs" section for the one-time
// `install -d` step) — and falls back to the user's own state directory
// otherwise. Either way the location is logged on the very first line
// so it's never a mystery which one is in effect.
package applog

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// SystemLogDir is the standard location Open prefers when writable.
const SystemLogDir = "/var/log/omavm"

// Open returns a JSON structured logger for component (e.g. "omavm",
// "omavm-gui") plus a close function to flush/release the underlying
// file.
//
// It deliberately does NOT call slog.SetDefault: gotk4 installs a GLib
// log writer that forwards every GTK/GDK-internal message (including
// noisy driver/Vulkan chatter unrelated to OmaVM) to whatever the
// process's default slog logger is. Replacing that default with our
// file-backed logger would flood it with library internals having
// nothing to do with OmaVM's own behavior. Callers that don't embed a
// GTK toolkit (the CLI) may reasonably call slog.SetDefault themselves
// with the returned logger; the GUI must not.
func Open(component string) (*slog.Logger, func() error, error) {
	dir, err := logDir()
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}

	path := filepath.Join(dir, component+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(f, nil))
	logger.Info("log opened", "component", component, "path", path)
	return logger, f.Close, nil
}

func logDir() (string, error) {
	if writable(SystemLogDir) {
		return SystemLogDir, nil
	}
	base, err := core.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "logs"), nil
}

func writable(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	probe := filepath.Join(dir, ".omavm-write-test")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(probe)
	return true
}
