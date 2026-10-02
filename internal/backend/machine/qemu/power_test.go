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
