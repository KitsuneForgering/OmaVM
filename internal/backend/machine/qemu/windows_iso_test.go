package qemu

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type windowsTransport func(*http.Request) (*http.Response, error)

func (f windowsTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestWindowsISOURLUsesMicrosoftPortugueseX64Link(t *testing.T) {
	client := windowsISOHTTP
	windowsISOHTTP = &http.Client{Transport: windowsTransport(func(req *http.Request) (*http.Response, error) {
		var body string
		switch {
		case req.URL.Path == "/en-us/software-download/windows11":
			body = `<option value="3813">Windows 11 (multi-edition ISO)</option>`
		case req.URL.Host == "vlscppe.microsoft.com":
			body = "ok"
		case strings.Contains(req.URL.Path, "getskuinformationbyproductedition"):
			if req.URL.Query().Get("ProductEditionId") != "3813" {
				t.Error("wrong edition ID")
			}
			body = `{"Skus":[{"Id":12,"LocalizedLanguage":"English International"},{"Id":34,"LocalizedLanguage":"Brazilian Portuguese"}]}`
		case strings.Contains(req.URL.Path, "GetProductDownloadLinksBySku"):
			if req.URL.Query().Get("SKU") != "34" {
				t.Error("wrong language SKU")
			}
			if req.Header.Get("Referer") != windowsDownloadPage {
				t.Error("missing Microsoft referer")
			}
			body = `{"ProductDownloadOptions":[{"Uri":"https://software-download.microsoft.com/Win11_x64.iso?token=example"}]}`
		default:
			t.Errorf("unexpected request %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { windowsISOHTTP = client })
	link, err := windowsISOURL(context.Background())
	if err != nil || link != "https://software-download.microsoft.com/Win11_x64.iso?token=example" {
		t.Fatalf("link = %q, %v", link, err)
	}
}

func TestWindowsISOURLExplainsMicrosoftBlock(t *testing.T) {
	client := windowsISOHTTP
	windowsISOHTTP = &http.Client{Transport: windowsTransport(func(req *http.Request) (*http.Response, error) {
		body := "ok"
		switch {
		case req.URL.Path == "/en-us/software-download/windows11":
			body = `<option value="3813">Windows 11</option>`
		case strings.Contains(req.URL.Path, "getskuinformationbyproductedition"):
			body = `{"Skus":[{"Id":34,"LocalizedLanguage":"Brazilian Portuguese"}]}`
		case strings.Contains(req.URL.Path, "GetProductDownloadLinksBySku"):
			body = `{"Errors":[{"Key":"ErrorSettings.SentinelReject","Value":"Sentinel marked this request as rejected."}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { windowsISOHTTP = client })
	_, err := windowsISOURL(context.Background())
	if err == nil || !strings.Contains(err.Error(), "download the ISO in a browser") {
		t.Fatalf("expected actionable Microsoft block error, got %v", err)
	}
}
