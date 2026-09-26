package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// fakeBackend is an in-memory Backend used to test Service without
// starting real containers or QEMU machines.
type fakeBackend struct {
	name    string
	created map[string]bool
	running map[string]bool
	removed map[string]bool
	execErr error
}

func newFakeBackend(name string) *fakeBackend {
	return &fakeBackend{
		name:    name,
		created: map[string]bool{},
		running: map[string]bool{},
		removed: map[string]bool{},
	}
}

func (f *fakeBackend) Name() string { return f.name }

func (f *fakeBackend) Create(ctx context.Context, env core.Environment) error {
	f.created[env.Name] = true
	return nil
}

func (f *fakeBackend) Start(ctx context.Context, env core.Environment) error {
	f.running[env.Name] = true
	return nil
}

func (f *fakeBackend) Open(ctx context.Context, env core.Environment) error {
	f.running[env.Name] = true
	return nil
}

func (f *fakeBackend) Stop(ctx context.Context, env core.Environment) error {
	f.running[env.Name] = false
	return nil
}

func (f *fakeBackend) Status(ctx context.Context, env core.Environment) (core.Status, error) {
	if f.running[env.Name] {
		return core.Status{State: core.StateRunning}, nil
	}
	return core.Status{State: core.StateStopped}, nil
}

func (f *fakeBackend) Exec(ctx context.Context, env core.Environment, args []string) error {
	return f.execErr
}

func (f *fakeBackend) Remove(ctx context.Context, env core.Environment) error {
	f.removed[env.Name] = true
	delete(f.created, env.Name)
	return nil
}

// memStore is an in-memory Store for tests.
type memStore struct {
	envs []core.Environment
}

func (m *memStore) Load() ([]core.Environment, error) {
	out := make([]core.Environment, len(m.envs))
	copy(out, m.envs)
	return out, nil
}

func (m *memStore) Save(envs []core.Environment) error {
	m.envs = envs
	return nil
}

func newTestService() (*core.Service, *fakeBackend, *fakeBackend) {
	box := newFakeBackend("fake-box")
	machine := newFakeBackend("fake-machine")
	svc := core.NewService(&memStore{}, box, machine)
	return svc, box, machine
}

func TestCreateStartStopStatus(t *testing.T) {
	ctx := context.Background()
	svc, box, _ := newTestService()

	env, err := svc.Create(ctx, core.Environment{Name: "radic", Image: "fedora", Kind: core.Box})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !box.created["radic"] {
		t.Fatal("expected backend Create to be called")
	}
	if env.Backend != "fake-box" {
		t.Fatalf("expected Backend to be set from backend.Name(), got %q", env.Backend)
	}
	if env.ID == "" {
		t.Fatal("expected a generated ID")
	}

	status, err := svc.Status(ctx, "radic")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != core.StateStopped {
		t.Fatalf("expected stopped before Start, got %s", status.State)
	}

	if err := svc.Start(ctx, "radic"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status, err = svc.Status(ctx, "radic")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != core.StateRunning {
		t.Fatalf("expected running after Start, got %s", status.State)
	}

	if err := svc.Stop(ctx, "radic"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	status, err = svc.Status(ctx, "radic")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != core.StateStopped {
		t.Fatalf("expected stopped after Stop, got %s", status.State)
	}
}

func TestCreateDuplicateNameFails(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService()

	if _, err := svc.Create(ctx, core.Environment{Name: "radic", Kind: core.Box}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ctx, core.Environment{Name: "radic", Kind: core.Box})
	if !errors.Is(err, core.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestOperationsOnUnknownNameFail(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService()

	if err := svc.Start(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := svc.Status(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := svc.Remove(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestKindWithoutBackendIsUnsupported(t *testing.T) {
	ctx := context.Background()
	svc := core.NewService(&memStore{}, newFakeBackend("fake-box"), nil)

	_, err := svc.Create(ctx, core.Environment{Name: "vm1", Kind: core.Machine})
	if !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestRemoveDropsFromStoreOnlyAfterBackendConfirms(t *testing.T) {
	ctx := context.Background()
	svc, box, _ := newTestService()

	if _, err := svc.Create(ctx, core.Environment{Name: "radic", Kind: core.Box}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Remove(ctx, "radic"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !box.removed["radic"] {
		t.Fatal("expected backend Remove to be called")
	}
	envs, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected no environments after Remove, got %v", envs)
	}
}

func TestExecPropagatesBackendError(t *testing.T) {
	ctx := context.Background()
	svc, box, _ := newTestService()
	box.execErr = errors.New("boom")

	if _, err := svc.Create(ctx, core.Environment{Name: "radic", Kind: core.Box}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Exec(ctx, "radic", []string{"go", "test", "./..."}); err == nil {
		t.Fatal("expected Exec error to propagate")
	}
}
