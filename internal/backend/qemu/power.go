package qemu

import (
	"os"
	"path/filepath"
	"strings"
)

// powerSupplyDir is overridable in tests; defaults to the real sysfs path.
var powerSupplyDir = "/sys/class/power_supply"

// onBattery reports whether the host currently has no AC/mains power
// connected, read directly from sysfs — no upower dependency needed for
// this one boolean. A host with no Mains power_supply entry at all
// (headless server, desktop with no battery) is treated as not on
// battery: Travel Mode only ever activates on clear evidence the host is
// unplugged, never as a guess.
func onBattery() bool {
	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil {
		return false
	}
	sawMains := false
	for _, entry := range entries {
		kind, err := os.ReadFile(filepath.Join(powerSupplyDir, entry.Name(), "type"))
		if err != nil || strings.TrimSpace(string(kind)) != "Mains" {
			continue
		}
		sawMains = true
		online, err := os.ReadFile(filepath.Join(powerSupplyDir, entry.Name(), "online"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(online)) == "1" {
			return false
		}
	}
	return sawMains
}
