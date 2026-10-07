package qemu

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// fakeQuickget installs a quickget that runs script, and returns its dir.
func fakeQuickget(t *testing.T, script string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "quickget")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := quickgetLookPath
	quickgetLookPath = func() (string, error) { return bin, nil }
	t.Cleanup(func() { quickgetLookPath = old })
}

func TestDownloadableImagesComeFromQuickget(t *testing.T) {
	fakeQuickget(t, `cat <<'CSV'
Display Name,OS,Release,Option,Downloader,PNG,SVG
Fedora,fedora,41,Workstation,curl,,
macOS,macos,sonoma,,curl,,
Ubuntu,ubuntu,24.04,,curl,,
CSV
`)
	b := &Backend{stateDir: t.TempDir()}
	images, err := b.DownloadableImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []core.DownloadableImage{{Name: "Fedora", OS: "fedora", Release: "41", Edition: "Workstation"}, {Name: "Ubuntu", OS: "ubuntu", Release: "24.04"}}
	if len(images) != 2 || images[0] != want[0] || images[1] != want[1] {
		t.Errorf("images = %+v (macOS needs more than an ISO and is left out)", images)
	}
}

func TestDownloadImageKeepsTheISOAndReportsRealProgress(t *testing.T) {
	// What quickget prints: a header, then curl's bar redrawn with \r.
	fakeQuickget(t, `[ "$1" = --download ] || exit 2
echo "Downloading Fedora $3 $4"
printf '##          10.0%%\r#####       50.5%%\r############ 100.0%%\n' >&2
echo iso > Fedora-$3.iso
`)
	b := &Backend{stateDir: t.TempDir()}
	images, _ := imagesDir()
	if err := os.MkdirAll(images, 0o755); err != nil {
		t.Fatal(err)
	}
	// One already there: never replaced.
	if err := os.WriteFile(filepath.Join(images, "Fedora-41.iso"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stages []string
	ctx := core.WithProgress(context.Background(), func(s string) { stages = append(stages, s) })
	path, err := b.DownloadImage(ctx, core.DownloadableImage{Name: "Fedora", OS: "fedora", Release: "41", Edition: "Workstation"})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(images, "Fedora-41 (2).iso") {
		t.Errorf("path = %q", path)
	}
	if mine, _ := os.ReadFile(filepath.Join(images, "Fedora-41.iso")); string(mine) != "mine" {
		t.Error("the download replaced an image that was already there")
	}
	if got := strings.Join(stages, "|"); !strings.Contains(got, ": 10%") || !strings.Contains(got, ": 50%") || !strings.Contains(got, ": 100%") {
		t.Errorf("progress = %q", got)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(images, ".download-*")); len(leftovers) != 0 {
		t.Errorf("work directories left behind: %v", leftovers)
	}
}

func TestDownloadThatIsNotAnISOIsRefused(t *testing.T) {
	fakeQuickget(t, `echo disk > image.qcow2`)
	b := &Backend{stateDir: t.TempDir()}
	_, err := b.DownloadImage(context.Background(), core.DownloadableImage{Name: "X", OS: "x", Release: "1"})
	if !errors.Is(err, core.ErrUnsupported) || !strings.Contains(err.Error(), "image.qcow2") {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadsNeedQuickget(t *testing.T) {
	old := quickgetLookPath
	quickgetLookPath = func() (string, error) { return "", os.ErrNotExist }
	defer func() { quickgetLookPath = old }()
	b := &Backend{stateDir: t.TempDir()}
	if _, err := b.DownloadableImages(context.Background()); !errors.Is(err, core.ErrUnsupported) {
		t.Errorf("err = %v", err)
	}
	if c := quickgetCapability(); c.Available || c.Hint == "" {
		t.Errorf("capability = %+v", c)
	}
}
