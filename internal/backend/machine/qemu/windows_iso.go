package qemu

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/KitsuneForgering/OmaVM/internal/core"
)

const windowsDownloadPage = "https://www.microsoft.com/en-us/software-download/windows11"
const windowsDownloadProfile = "606624d44113"
const windowsUserAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:100.0) Gecko/20100101 Firefox/100.0"

var windowsEditionID = regexp.MustCompile(`<option value="([0-9]{1,16})">Windows`)

var windowsISOHTTP = &http.Client{
	Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 30 * time.Second},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || !microsoftHost(req.URL.Hostname()) {
			return errors.New("Windows installer redirected outside Microsoft's HTTPS hosts")
		}
		return nil
	},
}

func microsoftHost(host string) bool {
	return host == "microsoft.com" || strings.HasSuffix(host, ".microsoft.com")
}

func windowsRequest(ctx context.Context, rawURL, referer string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", windowsUserAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	response, err := windowsISOHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Microsoft download service: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("Microsoft download response is unexpectedly large")
	}
	return data, nil
}

func microsoftAPIURL(endpoint string, query url.Values) string {
	return "https://www.microsoft.com/software-download-connector/api/" + endpoint + "?" + query.Encode()
}

func windowsISOURL(ctx context.Context) (string, error) {
	page, err := windowsRequest(ctx, windowsDownloadPage, "", 2<<20)
	if err != nil {
		return "", err
	}
	match := windowsEditionID.FindSubmatch(page)
	if match == nil {
		return "", errors.New("Microsoft's Windows 11 download page has no ISO edition; download it in a browser from " + windowsDownloadPage)
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	session := hex.EncodeToString(secret)
	permit := "https://vlscppe.microsoft.com/tags?" + url.Values{"org_id": {"y6jn8c31"}, "session_id": {session}}.Encode()
	if _, err := windowsRequest(ctx, permit, "", 100<<10); err != nil {
		return "", err
	}
	skuQuery := url.Values{"profile": {windowsDownloadProfile}, "ProductEditionId": {string(match[1])},
		"SKU": {"undefined"}, "friendlyFileName": {"undefined"}, "Locale": {"en-US"}, "sessionID": {session}}
	skuData, err := windowsRequest(ctx, microsoftAPIURL("getskuinformationbyproductedition", skuQuery), "", 200<<10)
	if err != nil {
		return "", err
	}
	var skuReply struct {
		Skus []struct {
			ID                json.Number `json:"Id"`
			LocalizedLanguage string      `json:"LocalizedLanguage"`
			Language          string      `json:"Language"`
		} `json:"Skus"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(skuData)))
	decoder.UseNumber()
	if err := decoder.Decode(&skuReply); err != nil {
		return "", fmt.Errorf("read Windows 11 languages: %w", err)
	}
	sku := ""
	for _, entry := range skuReply.Skus {
		if entry.LocalizedLanguage == "Brazilian Portuguese" || entry.Language == "Brazilian Portuguese" ||
			entry.LocalizedLanguage == "Portuguese (Brazil)" || entry.Language == "Portuguese (Brazil)" {
			sku = entry.ID.String()
			break
		}
	}
	if sku == "" {
		for _, entry := range skuReply.Skus {
			if entry.LocalizedLanguage == "English International" || entry.Language == "English International" {
				sku = entry.ID.String()
				break
			}
		}
	}
	if sku == "" || !regexp.MustCompile(`^[0-9]+$`).MatchString(sku) {
		return "", errors.New("Microsoft did not offer a supported Windows 11 language")
	}
	linkQuery := url.Values{"profile": {windowsDownloadProfile}, "productEditionId": {"undefined"},
		"SKU": {sku}, "friendlyFileName": {"undefined"}, "Locale": {"en-US"}, "sessionID": {session}}
	linkData, err := windowsRequest(ctx, microsoftAPIURL("GetProductDownloadLinksBySku", linkQuery), windowsDownloadPage, 200<<10)
	if err != nil {
		return "", err
	}
	if strings.Contains(string(linkData), "SentinelReject") || strings.Contains(string(linkData), "Sentinel marked this request as rejected") {
		return "", errors.New("Microsoft blocked the automatic Windows 11 ISO request; download the ISO in a browser from " + windowsDownloadPage + " and choose it in OmaVM")
	}
	var linkReply struct {
		Options []struct {
			URI string `json:"Uri"`
		} `json:"ProductDownloadOptions"`
	}
	if err := json.Unmarshal(linkData, &linkReply); err != nil {
		return "", fmt.Errorf("read Windows 11 download link: %w", err)
	}
	for _, option := range linkReply.Options {
		parsed, err := url.Parse(option.URI)
		if err == nil && parsed.Scheme == "https" && microsoftHost(parsed.Hostname()) &&
			strings.Contains(strings.ToLower(parsed.Path), "x64") && strings.HasSuffix(strings.ToLower(parsed.Path), ".iso") {
			return option.URI, nil
		}
	}
	return "", errors.New("Microsoft did not provide an x64 Windows 11 ISO link; download it in a browser from " + windowsDownloadPage)
}

func downloadWindowsISO(ctx context.Context) (string, error) {
	dir, err := imagesDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "windows-11.iso")
	if digest, err := os.ReadFile(dst + ".sha256"); err == nil {
		if want, decodeErr := hex.DecodeString(strings.TrimSpace(string(digest))); decodeErr == nil && len(want) == sha256.Size && sameFileHash(dst, want) {
			return dst, nil
		}
	}
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("%s already exists but was not downloaded by OmaVM; move it before downloading again", dst)
	}
	core.ReportProgress(ctx, "Asking Microsoft for the Windows 11 ISO")
	link, err := windowsISOURL(ctx)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", windowsUserAgent)
	response, err := windowsISOHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download Windows 11: HTTP %d", response.StatusCode)
	}
	const maxISO int64 = 10 << 30
	if response.ContentLength > maxISO {
		return "", errors.New("Windows 11 ISO is unexpectedly large")
	}
	partial, err := os.CreateTemp(dir, ".windows-download-")
	if err != nil {
		return "", err
	}
	defer os.Remove(partial.Name())
	defer partial.Close()
	hash := sha256.New()
	core.ReportProgress(ctx, "Downloading Windows 11")
	n, err := io.Copy(io.MultiWriter(partial, hash, &isoProgress{ctx: ctx, label: "Windows 11", total: response.ContentLength, next: 64 << 20}), io.LimitReader(response.Body, maxISO+1))
	if err != nil {
		return "", err
	}
	if n > maxISO || response.ContentLength >= 0 && n != response.ContentLength || n < 1<<30 {
		return "", errors.New("Windows 11 ISO download was incomplete or unexpectedly small")
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
	if err := os.WriteFile(dst+".sha256", []byte(hex.EncodeToString(hash.Sum(nil))+"\n"), 0o600); err != nil {
		_ = os.Remove(dst)
		return "", err
	}
	return dst, nil
}
