package core_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

func TestConcurrentCreatesPreserveRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "environments.json")
	results := make(chan error, 12)
	for i := 0; i < cap(results); i++ {
		go func(i int) {
			svc := core.NewService(&core.FileStore{Path: path}, newFakeBackend("box"), nil)
			_, err := svc.Create(context.Background(), core.Environment{Name: fmt.Sprintf("box-%d", i), Kind: core.Box})
			results <- err
		}(i)
	}
	for i := 0; i < cap(results); i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	envs, err := (&core.FileStore{Path: path}).Load()
	if err != nil || len(envs) != cap(results) {
		t.Fatalf("registry: %d environments, %v", len(envs), err)
	}
}

func TestRegistryLockCancellationAndRelease(t *testing.T) {
	store := &core.FileStore{Path: filepath.Join(t.TempDir(), "environments.json")}
	unlock, err := store.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = (&core.FileStore{Path: store.Path}).Lock(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock: %v", err)
	}
	unlock()
	release, err := store.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestDisconnectISOSettings(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	for _, kind := range []core.EnvironmentKind{core.Box, core.Machine} {
		name := kind.String()
		if _, err := svc.Create(ctx, core.Environment{Name: name, Kind: kind}); err != nil {
			t.Fatal(err)
		}
		for _, value := range []bool{true, false} {
			got, err := svc.Configure(ctx, name, core.SettingsPatch{DisconnectISO: &value})
			if kind == core.Box {
				if !errors.Is(err, core.ErrUnsupported) {
					t.Fatalf("Box media: %v", err)
				}
			} else if err != nil || got.DisconnectISO != value {
				t.Fatalf("Machine media: %+v, %v", got, err)
			}
		}
	}
}
