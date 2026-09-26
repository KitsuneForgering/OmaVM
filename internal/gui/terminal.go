package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// openInTerminal launches an interactive session for a Box by spawning
// the omavm CLI — never the container engine directly — inside the
// user's default terminal via xdg-terminal-exec. The GUI process has no
// controlling TTY to attach an interactive shell to, unlike a Machine's
// Open (which stays headless), so a Box needs a real terminal window
// here.
func openInTerminal(name string) error {
	omavmPath, err := findOmavmCLI()
	if err != nil {
		return err
	}
	cmd := exec.Command("xdg-terminal-exec", "--title="+name, "--", omavmPath, "open", name)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch terminal: %w", err)
	}
	return nil
}

func findOmavmCLI() (string, error) {
	if path, err := exec.LookPath("omavm"); err == nil {
		return path, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate omavm CLI: %w", err)
	}
	sibling := filepath.Join(filepath.Dir(self), "omavm")
	if _, err := os.Stat(sibling); err == nil {
		return sibling, nil
	}
	return "", fmt.Errorf("omavm CLI binary not found on PATH or next to %s", self)
}
