package qemu

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

func TestReadyDesktopChecksToolsBeforeDownloading(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	b := &Backend{stateDir: t.TempDir()}
	for _, image := range []string{"ready:ubuntu-24.04", "ready:fedora-44"} {
		err := b.createReadyDesktop(context.Background(), core.Environment{Image: image})
		if err == nil || !strings.Contains(err.Error(), "qemu-img") {
			t.Fatalf("%s: error = %v", image, err)
		}
	}
}

func TestReadySeedContainsTheSelectedDesktopSetup(t *testing.T) {
	for _, tool := range []string{"openssl", "mkfs.fat", "mcopy", "mtype"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	b := &Backend{stateDir: t.TempDir()}
	for _, tc := range []struct{ image, want string }{
		{core.ReadyUbuntuImage, "ubuntu-desktop-minimal"},
		{core.ReadyFedoraImage, "workstation-product-environment"},
	} {
		dir := b.dir(tc.image)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := b.makeReadySeed(context.Background(), tc.image, "abc123", tc.image); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("mtype", "-i", b.readySeedPath(tc.image, tc.image), "::user-data").Output()
		if err != nil || !strings.Contains(string(out), tc.want) {
			t.Fatalf("%s: seed content %q, error %v", tc.image, out, err)
		}
	}
}

func TestUbuntuImageHashRequiresTheExactSignedFile(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	manifest := []byte(strings.Repeat("cd", 32) + " *other-" + ubuntuCloudFile + "\n" +
		digest + " *" + ubuntuCloudFile + "\n")
	got, err := ubuntuImageHash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString(digest)
	if !bytes.Equal(got, want) {
		t.Fatalf("hash = %x, want %x", got, want)
	}
	if _, err := ubuntuImageHash([]byte(strings.Repeat("ab", 31) + " *" + ubuntuCloudFile)); err == nil {
		t.Fatal("accepted a truncated digest")
	}
}
