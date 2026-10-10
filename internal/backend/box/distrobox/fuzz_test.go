package distrobox

import (
	"encoding/hex"
	"regexp"
	"testing"
	"unicode/utf8"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// Podman's (and Docker's) rule for a container name.
var podmanName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// Whatever a Box is called, its container gets a name the engine accepts,
// and the status the engine reports for that name finds its way back:
// `ps` output joins a container's names with commas, so a comma in ours
// would split it.
func FuzzBoxNameReachesItsStatus(f *testing.F) {
	for _, seed := range []string{"dev", "dev, web", "Ação", "日本語", "-", "...", "a\tb", "🙂", "x,y,z"} {
		f.Add(seed, "0123456789abcdef")
	}
	f.Add("anything", "")
	f.Fuzz(func(t *testing.T, name, rawID string) {
		if !utf8.ValidString(name) || name == "" {
			return
		}
		// Core makes IDs of hex digits.
		id := hex.EncodeToString([]byte(rawID))
		box := boxName(core.Environment{Name: name, ID: id})
		if !podmanName.MatchString(box) {
			t.Fatalf("Box %q (id %q) got container name %q, which Podman refuses", name, id, box)
		}
		statuses := parseContainers(box + "\trunning\tUp 2 minutes\n")
		if statuses[box].State != core.StateRunning {
			t.Fatalf("container %q: status lost, parsed %v", box, statuses)
		}
	})
}
