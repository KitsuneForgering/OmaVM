package qemu

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// Guided Ubuntu, Fedora and Windows 11 downloads are native. Other systems
// come from quickget (Quickemu's downloader) when it is installed.

var quickgetLookPath = func() (string, error) { return exec.LookPath("quickget") }

// quickgetListTTL: listing every release takes quickget a while (some
// systems are looked up online), and the list changes over weeks.
const quickgetListTTL = 24 * time.Hour

func quickgetCapability() core.HostCapability {
	c := core.HostCapability{ID: "quickget", Label: "System downloads (quickget)"}
	if _, err := quickgetLookPath(); err == nil {
		c.Available, c.Detail = true, "Desktops can be created by downloading a system"
	} else {
		c.Detail = "quickget is not installed"
		c.Hint = "Install quickemu (AUR) to download a system while creating a Desktop"
	}
	return c
}

func errNoQuickget() error {
	return core.Unsupportedf("downloading a system needs quickget: install quickemu (AUR), or choose an ISO you already have")
}

func (b *Backend) DownloadableImages(ctx context.Context) ([]core.DownloadableImage, error) {
	path, err := quickgetLookPath()
	if err != nil {
		return nil, errNoQuickget()
	}
	cache := filepath.Join(b.stateDir, "quickget-list.csv")
	data, err := os.ReadFile(cache)
	if info, statErr := os.Stat(cache); err != nil || statErr != nil || time.Since(info.ModTime()) > quickgetListTTL {
		out, runErr := exec.CommandContext(ctx, path, "--list-csv").Output()
		if runErr != nil {
			return nil, fmt.Errorf("quickget --list-csv: %w", runErr)
		}
		data = out
		_ = os.MkdirAll(b.stateDir, 0o755)
		_ = os.WriteFile(cache, data, 0o644)
	}
	return parseQuickgetList(data)
}

// parseQuickgetList reads `quickget --list-csv`: "Display Name,OS,Release,
// Option,Downloader,PNG,SVG". macOS is left out: it needs OpenCore and
// more than an ISO to boot.
func parseQuickgetList(data []byte) ([]core.DownloadableImage, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read quickget's list: %w", err)
	}
	var images []core.DownloadableImage
	for i, row := range rows {
		if i == 0 || len(row) < 4 || row[1] == "macos" {
			continue
		}
		images = append(images, core.DownloadableImage{Name: row[0], OS: row[1], Release: row[2], Edition: row[3]})
	}
	return images, nil
}

// imagesDir is where downloaded systems are kept, next to the Machines'
// links and the shared folder.
func imagesDir() (string, error) {
	dir, err := hostLinkDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Images"), nil
}

var curlPercent = regexp.MustCompile(`(\d+(?:\.\d+)?)%`)

func (b *Backend) DownloadImage(ctx context.Context, image core.DownloadableImage) (string, error) {
	if image.OS == "ubuntu" && image.Release == "24.04" && image.Edition == "" {
		return downloadOfficialISO(ctx, "Ubuntu 24.04 LTS", ubuntuISOURL, ubuntuISOFile, ubuntuISOSHA)
	}
	if image.OS == "fedora" && image.Release == "44" && image.Edition == "Workstation" {
		return downloadOfficialISO(ctx, "Fedora 44 Workstation", fedoraISOURL, fedoraISOFile, fedoraISOSHA)
	}
	if image.OS == "windows" && image.Release == "11" && image.Edition == "" {
		return downloadWindowsISO(ctx)
	}
	path, err := quickgetLookPath()
	if err != nil {
		return "", errNoQuickget()
	}
	dir, err := imagesDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	// quickget writes into its working directory, named after the URL: an
	// empty directory of its own tells which file it was.
	work, err := os.MkdirTemp(dir, ".download-")
	if err != nil {
		return "", fmt.Errorf("prepare the download: %w", err)
	}
	defer os.RemoveAll(work)

	name := strings.TrimSpace(strings.Join([]string{image.Name, image.Release, image.Edition}, " "))
	args := []string{"--download", image.OS, image.Release}
	if image.Edition != "" {
		args = append(args, image.Edition)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = work
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout // curl's progress bar goes to stderr
	core.ReportProgress(ctx, "Downloading %s", name)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start quickget: %w", err)
	}
	// curl redraws its bar with \r: split on both, and report each whole
	// percent it actually printed.
	scanner := bufio.NewScanner(stdout)
	scanner.Split(splitLinesAndReturns)
	var last []string
	shown := -1
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if m := curlPercent.FindStringSubmatch(line); m != nil && strings.Contains(line, "#") {
			if pct, err := strconv.ParseFloat(m[1], 64); err == nil && int(pct) != shown {
				shown = int(pct)
				core.ReportProgress(ctx, "Downloading %s: %d%%", name, shown)
			}
			continue
		}
		last = append(last, line)
		if len(last) > 5 {
			last = last[1:]
		}
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("downloading %s failed: %s", name, strings.Join(last, "; "))
	}

	entries, err := os.ReadDir(work)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".iso") {
			continue
		}
		dst := freePath(dir, e.Name())
		if err := os.Rename(filepath.Join(work, e.Name()), dst); err != nil {
			return "", fmt.Errorf("keep the download: %w", err)
		}
		return dst, nil
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if len(got) == 0 {
		return "", errors.New("quickget finished without downloading anything")
	}
	return "", core.Unsupportedf("%s doesn't come as an installation ISO (got %s); a Desktop boots from an ISO", name, strings.Join(got, ", "))
}

// freePath is dir/name, or "name (2).ext"... when that is taken: a
// download never replaces an image already there.
func freePath(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)
	for n := 2; ; n++ {
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
	}
}

func splitLinesAndReturns(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
