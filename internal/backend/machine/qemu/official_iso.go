package qemu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// Pinned official installer media. Update the URL and digest together when
// either distribution publishes a new point release.
const (
	ubuntuISOFile = "ubuntu-24.04.5.1-desktop-amd64.iso"
	ubuntuISOURL  = "https://releases.ubuntu.com/24.04/" + ubuntuISOFile
	ubuntuISOSHA  = "4da4a0c9035da8e68a59a838674f403f0a54472c78a83b4fb7f78d03588f85a7"
	fedoraISOFile = "Fedora-Workstation-Live-44-1.7.x86_64.iso"
	fedoraISOURL  = "https://dl.fedoraproject.org/pub/fedora/linux/releases/44/Workstation/x86_64/iso/" + fedoraISOFile
	fedoraISOSHA  = "1620295f6a00c27c3208f0c00b8ece4eab1ec69b9002152d97488bf26a426ddf"
)

var officialISOHTTP = &http.Client{
	Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 30 * time.Second},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" {
			return errors.New("installer download redirected outside HTTPS")
		}
		return nil
	},
}

func downloadOfficialISO(ctx context.Context, label, url, filename, digest string) (string, error) {
	dir, err := imagesDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	want, err := hex.DecodeString(digest)
	if err != nil || len(want) != sha256.Size {
		return "", errors.New("invalid installer checksum")
	}
	dst := filepath.Join(dir, filename)
	if sameFileHash(dst, want) {
		return dst, nil
	}
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("%s already exists with a different checksum; move it before downloading again", dst)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := officialISOHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", label, response.StatusCode)
	}
	const maxISO int64 = 10 << 30
	if response.ContentLength > maxISO {
		return "", errors.New("installer ISO is unexpectedly large")
	}
	partial, err := os.CreateTemp(dir, ".iso-download-")
	if err != nil {
		return "", err
	}
	defer os.Remove(partial.Name())
	defer partial.Close()
	hash := sha256.New()
	progress := &isoProgress{ctx: ctx, label: label, total: response.ContentLength, next: 64 << 20}
	core.ReportProgress(ctx, "Downloading %s", label)
	n, err := io.Copy(io.MultiWriter(partial, hash, progress), io.LimitReader(response.Body, maxISO+1))
	if err != nil {
		return "", err
	}
	if n > maxISO || response.ContentLength >= 0 && n != response.ContentLength {
		return "", errors.New("installer ISO download was incomplete or too large")
	}
	if !bytes.Equal(hash.Sum(nil), want) {
		return "", errors.New("installer ISO checksum did not match the official release")
	}
	if err := partial.Sync(); err != nil {
		return "", err
	}
	if err := partial.Close(); err != nil {
		return "", err
	}
	if err := os.Link(partial.Name(), dst); err != nil {
		return "", err
	}
	return dst, nil
}

type isoProgress struct {
	ctx               context.Context
	label             string
	total, done, next int64
}

func (p *isoProgress) Write(data []byte) (int, error) {
	p.done += int64(len(data))
	if p.done >= p.next {
		if p.total > 0 {
			core.ReportProgress(p.ctx, "Downloading %s: %d%%", p.label, 100*p.done/p.total)
		} else {
			core.ReportProgress(p.ctx, "Downloading %s: %d MiB", p.label, p.done>>20)
		}
		p.next += 64 << 20
	}
	return len(data), nil
}
