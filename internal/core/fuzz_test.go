package core

import (
	"os"
	"path/filepath"
	"testing"
)

// Every name Create accepts must come back from the registry exactly as
// typed: the registry is JSON, and a name that changed on the way (bytes
// that aren't UTF-8 become U+FFFD) could never be looked up again.
func FuzzAcceptedNameSurvivesTheRegistry(f *testing.F) {
	for _, seed := range []string{"Fedora", "Windows 11, trabalho", "Ação", "a\"b`c$d\\e", "日本語", "x\xffy", "emoji 🙂", "%u %%"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if validateEnvironmentName(name) != nil {
			return
		}
		store := &FileStore{Path: filepath.Join(t.TempDir(), "environments.json")}
		if err := store.Save([]Environment{{ID: "id", Name: name, Kind: Machine}}); err != nil {
			t.Fatal(err)
		}
		envs, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if len(envs) != 1 || envs[0].Name != name {
			t.Fatalf("accepted name %q came back as %q", name, envs[0].Name)
		}
	})
}

// A damaged or hand-edited registry is reported, never a crash.
func FuzzRegistryLoadNeverPanics(f *testing.F) {
	for _, seed := range []string{`[]`, `[{"id":"a","name":"n","kind":1}]`, `{"version":1,"environments":[]}`, `{"version":99}`, `{`, ``, `null`, `[null]`, `{"environments":"x"}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		store := &FileStore{Path: filepath.Join(t.TempDir(), "environments.json")}
		if err := os.WriteFile(store.Path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		envs, err := store.Load()
		if err != nil {
			return
		}
		// What loads must save and load back the same.
		if err := store.Save(envs); err != nil {
			t.Fatal(err)
		}
		again, err := store.Load()
		if err != nil || len(again) != len(envs) {
			t.Fatalf("a loaded registry didn't survive a save: %v (%d vs %d)", err, len(again), len(envs))
		}
	})
}
