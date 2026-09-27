package qemu

import (
	"os"
	"path/filepath"
	"testing"
)

func writePowerSupply(t *testing.T, dir, name, kind, online string) {
	t.Helper()
	supplyDir := filepath.Join(dir, name)
	if err := os.MkdirAll(supplyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(supplyDir, "type"), []byte(kind+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if online != "" {
		if err := os.WriteFile(filepath.Join(supplyDir, "online"), []byte(online+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOnBattery(t *testing.T) {
	orig := powerSupplyDir
	defer func() { powerSupplyDir = orig }()

	t.Run("no power_supply info", func(t *testing.T) {
		powerSupplyDir = filepath.Join(t.TempDir(), "missing")
		if onBattery() {
			t.Fatal("expected false when there is no power_supply information")
		}
	})

	t.Run("plugged in", func(t *testing.T) {
		dir := t.TempDir()
		writePowerSupply(t, dir, "BAT0", "Battery", "")
		writePowerSupply(t, dir, "AC", "Mains", "1")
		powerSupplyDir = dir
		if onBattery() {
			t.Fatal("expected false when Mains is online")
		}
	})

	t.Run("on battery", func(t *testing.T) {
		dir := t.TempDir()
		writePowerSupply(t, dir, "BAT0", "Battery", "")
		writePowerSupply(t, dir, "AC", "Mains", "0")
		powerSupplyDir = dir
		if !onBattery() {
			t.Fatal("expected true when Mains is offline")
		}
	})

	t.Run("desktop with no battery hardware", func(t *testing.T) {
		dir := t.TempDir()
		powerSupplyDir = dir
		if onBattery() {
			t.Fatal("expected false with no power_supply entries at all")
		}
	})
}
