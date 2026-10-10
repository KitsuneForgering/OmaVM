package qemu

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

const (
	ubuntuCloudBase        = "https://cloud-images.ubuntu.com/noble/current/"
	ubuntuCloudFile        = "noble-server-cloudimg-amd64.img"
	ubuntuCloudKey         = "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0xD2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81"
	ubuntuCloudFingerprint = "D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81"
)

var ubuntuHTTP = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment,
	ResponseHeaderTimeout: 30 * time.Second}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 || req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
		return errors.New("image download redirected outside its official HTTPS host")
	}
	return nil
}}

func (b *Backend) readySeedPath(name, image string) string {
	if image == core.ReadyFedoraImage {
		return filepath.Join(b.dir(name), "fedora-seed.img")
	}
	return filepath.Join(b.dir(name), "ubuntu-seed.img")
}
func (b *Backend) readyCredentialsPath(name string) string {
	return filepath.Join(b.dir(name), "ubuntu-credentials.txt")
}

func (b *Backend) Credentials(_ context.Context, env core.Environment) (string, error) {
	if !core.IsReadyImage(env.Image) {
		return "", core.Unsupportedf("%s has no generated login", env.Name)
	}
	data, err := os.ReadFile(b.readyCredentialsPath(b.key(env)))
	if err != nil {
		return "", fmt.Errorf("read initial login: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (b *Backend) createReadyDesktop(ctx context.Context, env core.Environment) (err error) {
	// Fail before the image download if the host cannot write the first-boot
	// disk. This was previously discovered only after fetching 600 MiB.
	for _, tool := range []string{"qemu-img", "openssl", "mkfs.fat", "mcopy"} {
		if _, lookupErr := exec.LookPath(tool); lookupErr != nil {
			return fmt.Errorf("assisted installation needs %s: %w", tool, lookupErr)
		}
	}
	if env.Image == core.ReadyUbuntuImage {
		if _, lookupErr := exec.LookPath("gpgv"); lookupErr != nil {
			return fmt.Errorf("assisted installation needs gpgv: %w", lookupErr)
		}
		if _, lookupErr := exec.LookPath("gpg"); lookupErr != nil {
			return fmt.Errorf("assisted installation needs gpg: %w", lookupErr)
		}
	}
	name := b.key(env)
	disk := b.diskPath(name)
	seed := b.readySeedPath(name, env.Image)
	credentials := b.readyCredentialsPath(name)
	defer func() {
		if err != nil {
			_ = os.Remove(disk)
			_ = os.Remove(seed)
			_ = os.Remove(credentials)
		}
	}()
	var image string
	if env.Image == core.ReadyFedoraImage {
		image, err = downloadFedoraCloud(ctx)
	} else {
		image, err = downloadUbuntuCloud(ctx)
	}
	if err != nil {
		return err
	}
	core.ReportProgress(ctx, "Preparing desktop disk")
	// The signed upstream image is already a standalone compressed qcow2.
	// Copying it keeps its compression; qemu-img convert expanded it from
	// roughly 600 MiB to more than 3 GiB before the guest wrote anything.
	if err := copyFile(image, disk, 0o600); err != nil {
		return fmt.Errorf("copy desktop disk: %w", err)
	}
	if out, e := runQEMU(ctx, "qemu-img", "resize", disk, fmt.Sprint(diskSize)); e != nil {
		return qemuErr("resize desktop disk", e, out)
	}
	if err = b.makeReadySeed(ctx, name, env.ID, env.Image); err != nil {
		return err
	}
	return b.setUpFirmware(name)
}

func downloadUbuntuCloud(ctx context.Context) (string, error) {
	dir, err := imagesDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	work, err := os.MkdirTemp(dir, ".ubuntu-verify-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	core.ReportProgress(ctx, "Checking Ubuntu image signature")
	key, err := fetchUbuntu(ctx, ubuntuCloudKey, 100<<10)
	if err != nil {
		return "", fmt.Errorf("Ubuntu signing key: %w", err)
	}
	show := exec.CommandContext(ctx, "gpg", "--batch", "--homedir", work, "--show-keys", "--with-colons")
	show.Stdin = bytes.NewReader(key)
	fingerprints, err := show.Output()
	if err != nil || !strings.Contains(string(fingerprints), "fpr:::::::::"+ubuntuCloudFingerprint+":") {
		return "", errors.New("Ubuntu cloud signing key fingerprint did not match the trusted key")
	}
	keyring := filepath.Join(work, "ubuntu.gpg")
	importKey := exec.CommandContext(ctx, "gpg", "--batch", "--no-default-keyring", "--homedir", work, "--keyring", keyring, "--import")
	importKey.Stdin = bytes.NewReader(key)
	if out, err := importKey.CombinedOutput(); err != nil {
		return "", fmt.Errorf("import Ubuntu signing key: %w: %s", err, out)
	}
	sums, err := fetchUbuntu(ctx, ubuntuCloudBase+"SHA256SUMS", 128<<10)
	if err != nil {
		return "", err
	}
	sig, err := fetchUbuntu(ctx, ubuntuCloudBase+"SHA256SUMS.gpg", 32<<10)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(work, "SHA256SUMS"), sums, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(work, "SHA256SUMS.gpg"), sig, 0o600); err != nil {
		return "", err
	}
	verify := exec.CommandContext(ctx, "gpgv", "--keyring", keyring, filepath.Join(work, "SHA256SUMS.gpg"), filepath.Join(work, "SHA256SUMS"))
	if out, err := verify.CombinedOutput(); err != nil {
		return "", fmt.Errorf("Ubuntu image manifest signature failed: %w: %s", err, out)
	}
	want, err := ubuntuImageHash(sums)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "ubuntu-24.04-"+hex.EncodeToString(want[:8])+".qcow2")
	if sameFileHash(dst, want) {
		return dst, nil
	}
	core.ReportProgress(ctx, "Downloading Ubuntu 24.04 LTS")
	partial, err := os.CreateTemp(dir, ".ubuntu-download-")
	if err != nil {
		return "", err
	}
	defer os.Remove(partial.Name())
	defer partial.Close()
	response, err := ubuntuRequest(ctx, ubuntuCloudBase+ubuntuCloudFile)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.ContentLength > 2<<30 {
		return "", errors.New("Ubuntu image is unexpectedly large")
	}
	hash := sha256.New()
	progress := &ubuntuProgress{ctx: ctx, total: response.ContentLength, next: 50 << 20}
	n, err := io.Copy(io.MultiWriter(partial, hash, progress), io.LimitReader(response.Body, (2<<30)+1))
	if err != nil {
		return "", err
	}
	if n > 2<<30 || (response.ContentLength >= 0 && n != response.ContentLength) {
		return "", errors.New("Ubuntu image download was incomplete or too large")
	}
	if !bytes.Equal(hash.Sum(nil), want) {
		return "", errors.New("Ubuntu image checksum did not match its signed manifest")
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

func fetchUbuntu(ctx context.Context, url string, max int64) ([]byte, error) {
	r, err := ubuntuRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("Ubuntu verification file is unexpectedly large")
	}
	return data, nil
}

func ubuntuRequest(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	r, err := ubuntuHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if r.StatusCode != http.StatusOK {
		r.Body.Close()
		return nil, fmt.Errorf("Ubuntu download: HTTP %d", r.StatusCode)
	}
	return r, nil
}

func ubuntuImageHash(sums []byte) ([]byte, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != ubuntuCloudFile {
			continue
		}
		hash, err := hex.DecodeString(fields[0])
		if err == nil && len(hash) == sha256.Size {
			return hash, nil
		}
		break
	}
	return nil, errors.New("signed Ubuntu manifest has no SHA256 for the amd64 cloud image")
}

func sameFileHash(path string, want []byte) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	return err == nil && bytes.Equal(h.Sum(nil), want)
}

type ubuntuProgress struct {
	ctx               context.Context
	total, done, next int64
}

func (p *ubuntuProgress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.done >= p.next {
		if p.total > 0 {
			core.ReportProgress(p.ctx, "Downloading Ubuntu 24.04 LTS: %d%%", 100*p.done/p.total)
		}
		p.next += 50 << 20
	}
	return len(b), nil
}

func (b *Backend) makeReadySeed(ctx context.Context, name, id, image string) error {
	secret := make([]byte, 15)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	password := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	hasher := exec.CommandContext(ctx, "openssl", "passwd", "-6", "-stdin")
	hasher.Stdin = strings.NewReader(password + "\n")
	hashed, err := hasher.Output()
	if err != nil {
		return fmt.Errorf("hash Ubuntu password: %w", err)
	}
	userData := fmt.Sprintf(`#cloud-config
users:
  - name: omavm
    gecos: OmaVM
    groups: [adm, sudo]
    shell: /bin/bash
    lock_passwd: false
    passwd: '%s'
ssh_pwauth: false
package_update: true
packages:
  - ubuntu-desktop-minimal
  - qemu-guest-agent
  - spice-vdagent
runcmd:
  - [sh, -c, "test -x /usr/sbin/gdm3 && touch /var/lib/omavm-desktop-ready"]
power_state:
  mode: reboot
  message: Desktop installation finished
  timeout: 7200
  condition: test -f /var/lib/omavm-desktop-ready
`, strings.TrimSpace(string(hashed)))
	if image == core.ReadyFedoraImage {
		userData = fmt.Sprintf(`#cloud-config
users:
  - name: omavm
    gecos: OmaVM
    groups: [wheel]
    shell: /bin/bash
    lock_passwd: false
    passwd: '%s'
ssh_pwauth: false
runcmd:
  - [sh, -c, "dnf -y environment install workstation-product-environment && dnf -y install qemu-guest-agent spice-vdagent && systemctl set-default graphical.target && systemctl enable gdm.service && touch /var/lib/omavm-desktop-ready"]
power_state:
  mode: reboot
  message: Desktop installation finished
  timeout: 7200
  condition: test -f /var/lib/omavm-desktop-ready
`, strings.TrimSpace(string(hashed)))
	}
	work, err := os.MkdirTemp(b.dir(name), ".ready-seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if err := os.WriteFile(filepath.Join(work, "user-data"), []byte(userData), 0o600); err != nil {
		return err
	}
	hostname := "ubuntu"
	if image == core.ReadyFedoraImage {
		hostname = "fedora"
	}
	meta := fmt.Sprintf("instance-id: omavm-%s\nlocal-hostname: %s\n", id, hostname)
	if err := os.WriteFile(filepath.Join(work, "meta-data"), []byte(meta), 0o600); err != nil {
		return err
	}
	seed := b.readySeedPath(name, image)
	if out, err := exec.CommandContext(ctx, "mkfs.fat", "-C", "-n", "CIDATA", seed, "4096").CombinedOutput(); err != nil {
		return fmt.Errorf("create first-boot disk (install dosfstools): %w: %s", err, out)
	}
	if err := os.Chmod(seed, 0o600); err != nil {
		return err
	}
	for _, file := range []string{"user-data", "meta-data"} {
		if out, err := exec.CommandContext(ctx, "mcopy", "-i", seed, filepath.Join(work, file), "::"+file).CombinedOutput(); err != nil {
			return fmt.Errorf("write first-boot disk (install mtools): %w: %s", err, out)
		}
	}
	return os.WriteFile(b.readyCredentialsPath(name), []byte("Username: omavm\nPassword: "+password+"\n"), 0o600)
}
