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

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

// Fedora Cloud Base 44, release 1.7, x86_64. The digest is published on
// https://fedoraproject.org/cloud/download/ and pins this exact image.
const (
	fedoraCloudFile = "Fedora-Cloud-Base-Generic-44-1.7.x86_64.qcow2"
	fedoraCloudURL  = "https://dl.fedoraproject.org/pub/fedora/linux/releases/44/Cloud/x86_64/images/" + fedoraCloudFile
	fedoraCloudSHA  = "28680fe5b371a5a82ebf43a31926e086a168e59949d03969c5093e7071f90b7f"
)

func downloadFedoraCloud(ctx context.Context) (string, error) {
	dir, err := imagesDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	want, _ := hex.DecodeString(fedoraCloudSHA)
	dst := filepath.Join(dir, "fedora-44-1.7-"+fedoraCloudSHA[:16]+".qcow2")
	if sameFileHash(dst, want) {
		return dst, nil
	}
	core.ReportProgress(ctx, "Downloading Fedora 44")
	partial, err := os.CreateTemp(dir, ".fedora-download-")
	if err != nil {
		return "", err
	}
	defer os.Remove(partial.Name())
	defer partial.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fedoraCloudURL, nil)
	if err != nil {
		return "", err
	}
	response, err := ubuntuHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Fedora download: HTTP %d", response.StatusCode)
	}
	const maxImage = 2 << 30
	if response.ContentLength > maxImage {
		return "", errors.New("Fedora image is unexpectedly large")
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(partial, hash), io.LimitReader(response.Body, maxImage+1))
	if err != nil {
		return "", err
	}
	if n > maxImage || response.ContentLength >= 0 && n != response.ContentLength {
		return "", errors.New("Fedora image download was incomplete or too large")
	}
	if !bytes.Equal(hash.Sum(nil), want) {
		return "", errors.New("Fedora image checksum did not match the published digest")
	}
	if err := partial.Sync(); err != nil {
		return "", err
	}
	if err := partial.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(partial.Name(), dst); err != nil {
		return "", err
	}
	return dst, nil
}
