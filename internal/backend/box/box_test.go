package box

import (
	"context"
	"testing"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

type fakeBackend struct {
	name  string
	calls []string
}

func (f *fakeBackend) Name() string { return f.name }
func (f *fakeBackend) record(call string) error {
	f.calls = append(f.calls, call)
	return nil
}
func (f *fakeBackend) Create(context.Context, core.Environment) error { return f.record("create") }
func (f *fakeBackend) Start(context.Context, core.Environment) error  { return f.record("start") }
func (f *fakeBackend) Open(context.Context, core.Environment) error   { return f.record("open") }
func (f *fakeBackend) Stop(context.Context, core.Environment) error   { return f.record("stop") }
func (f *fakeBackend) Status(context.Context, core.Environment) (core.Status, error) {
	f.calls = append(f.calls, "status")
	return core.Status{}, nil
}
func (f *fakeBackend) Exec(context.Context, core.Environment, []string) error {
	return f.record("exec")
}
func (f *fakeBackend) Remove(context.Context, core.Environment) error { return f.record("remove") }

func TestNewBoxesUseDistrobox(t *testing.T) {
	primary := &fakeBackend{name: "distrobox"}
	legacy := &fakeBackend{name: "podman"}
	b := New(primary, legacy)

	if err := b.Create(context.Background(), core.Environment{Name: "new"}); err != nil {
		t.Fatal(err)
	}
	if len(primary.calls) != 1 || primary.calls[0] != "create" || len(legacy.calls) != 0 {
		t.Fatalf("unexpected routing: primary=%v legacy=%v", primary.calls, legacy.calls)
	}
}

func TestLegacyBoxesKeepTheirContainerBackend(t *testing.T) {
	for _, backendName := range []string{"podman", "docker"} {
		t.Run(backendName, func(t *testing.T) {
			primary := &fakeBackend{name: "distrobox"}
			legacy := &fakeBackend{name: backendName}
			b := New(primary, legacy)

			if err := b.Start(context.Background(), core.Environment{Backend: backendName}); err != nil {
				t.Fatal(err)
			}
			if len(legacy.calls) != 1 || legacy.calls[0] != "start" || len(primary.calls) != 0 {
				t.Fatalf("unexpected routing: primary=%v legacy=%v", primary.calls, legacy.calls)
			}
		})
	}
}
