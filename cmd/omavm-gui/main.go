// Command omavm-gui is the Experience Center: a graphical front end
// that wires the same core.Service the CLI uses and contains no
// environment-management logic of its own (CLAUDE.md CLI/GUI Contract).
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/KitsuneSemCalda/OmaVM/internal/applog"
	"github.com/KitsuneSemCalda/OmaVM/internal/backend/container"
	"github.com/KitsuneSemCalda/OmaVM/internal/backend/qemu"
	"github.com/KitsuneSemCalda/OmaVM/internal/core"
	"github.com/KitsuneSemCalda/OmaVM/internal/gui"
)

func main() {
	// Not installed as the slog default here: gotk4 forwards GTK/GDK's
	// own internal log chatter to slog.Default(), and pointing that at
	// our file would flood it with library internals. gui.Run takes
	// the logger explicitly instead.
	logger, closeLog, err := applog.Open("omavm-gui")
	if err != nil {
		fmt.Fprintln(os.Stderr, "omavm-gui: warning: logging disabled:", err)
		logger = slog.New(slog.DiscardHandler)
	} else {
		defer closeLog()
	}

	qemuBackend, err := qemu.New()
	if err != nil {
		logger.Error("startup failed", "error", err)
		fmt.Fprintln(os.Stderr, "omavm-gui:", err)
		os.Exit(1)
	}
	store, err := core.NewFileStore()
	if err != nil {
		logger.Error("startup failed", "error", err)
		fmt.Fprintln(os.Stderr, "omavm-gui:", err)
		os.Exit(1)
	}
	svc := core.NewService(store, container.New(), qemuBackend)
	logger.Info("starting Experience Center")
	gui.Run(svc, logger)
}
