package qemu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestOfficialISOChecksAndCachesDownload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	data := []byte("test ISO payload")
	sum := sha256.Sum256(data)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()
	client := officialISOHTTP
	officialISOHTTP = server.Client()
	t.Cleanup(func() { officialISOHTTP = client })
	file := "test.iso"
	path, err := downloadOfficialISO(context.Background(), "test", server.URL, file, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != file {
		t.Fatalf("download path = %q", path)
	}
	server.Close()
	if cached, err := downloadOfficialISO(context.Background(), "test", server.URL, file, hex.EncodeToString(sum[:])); err != nil || cached != path {
		t.Fatalf("verified cache: %q, %v", cached, err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := downloadOfficialISO(context.Background(), "test", server.URL, file, hex.EncodeToString(sum[:])); err == nil {
		t.Fatal("accepted an existing ISO with a wrong checksum")
	}
}
