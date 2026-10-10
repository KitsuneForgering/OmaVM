package qemu

import (
	"errors"
	"os"
	"testing"
)

// The list is polled every 3 s; where the guest mounted the shared folder
// is found by running a command in the guest, so one QEMU session asks
// once per mountCacheTTL, and a new session (another pid) asks again.
func TestMountCheckIsCachedPerSession(t *testing.T) {
	b := &Backend{stateDir: t.TempDir()}
	if err := os.MkdirAll(b.dir("m"), 0o700); err != nil {
		t.Fatal(err)
	}
	setPID := func(pid string) {
		if err := os.WriteFile(b.pidPath("m"), []byte(pid), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	check := func() (string, error) { calls++; return "/mnt/omavm-share", nil }

	setPID("100")
	for range 3 {
		if mount, err := b.cachedMount("m", check); err != nil || mount != "/mnt/omavm-share" {
			t.Fatalf("got %q, %v", mount, err)
		}
	}
	if calls != 1 {
		t.Fatalf("guest asked %d times in one session, want 1", calls)
	}

	setPID("200")
	if _, err := b.cachedMount("m", check); err != nil || calls != 2 {
		t.Fatalf("a new session must ask again: calls %d, err %v", calls, err)
	}

	b.forgetMount("m")
	if _, err := b.cachedMount("m", check); err != nil || calls != 3 {
		t.Fatalf("after forgetMount the guest must be asked: calls %d, err %v", calls, err)
	}

	b.forgetMount("m")
	failing := func() (string, error) { calls++; return "", errors.New("agent gone") }
	_, _ = b.cachedMount("m", failing)
	if _, err := b.cachedMount("m", check); err != nil || calls != 5 {
		t.Fatalf("a failed check must not be cached: calls %d, err %v", calls, err)
	}
}
