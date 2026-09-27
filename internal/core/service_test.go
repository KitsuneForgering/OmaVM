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

func (m *memStore) Lock(context.Context) (func(), error) { return func() {}, nil }

func (m *memStore) Load() ([]core.Environment, error) {
	out := make([]core.Environment, len(m.envs))
	copy(out, m.envs)
	return out, nil
}

func (m *memStore) Save(envs []core.Environment) error {
	m.envs = envs
	return nil
}

// fakeSnapshotBackend adds SnapshotManager on top of fakeBackend so tests
// can exercise a backend that supports snapshots (e.g. Machine/QEMU)
// separately from one that doesn't (e.g. Box today).
type fakeSnapshotBackend struct {
	*fakeBackend
	snapshots map[string]bool
	createErr error
	removeErr error
}

func newFakeSnapshotBackend(name string) *fakeSnapshotBackend {
	return &fakeSnapshotBackend{fakeBackend: newFakeBackend(name), snapshots: map[string]bool{}}
}

func (f *fakeSnapshotBackend) CreateSnapshot(ctx context.Context, env core.Environment, tag string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.snapshots[tag] = true
	return nil
}

func (f *fakeSnapshotBackend) GoToSnapshot(ctx context.Context, env core.Environment, tag string) error {
	if !f.snapshots[tag] {
		return errors.New("unknown snapshot")
	}
	return nil
}

func (f *fakeSnapshotBackend) RemoveSnapshot(ctx context.Context, env core.Environment, tag string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	if !f.snapshots[tag] {
		return errors.New("unknown snapshot")
	}
	delete(f.snapshots, tag)
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

func TestConfigureMachineSettings(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService()
	if _, err := svc.Create(ctx, core.Environment{Name: "desktop", Image: "system.iso", Kind: core.Machine}); err != nil {
		t.Fatal(err)
	}
	initial, err := svc.Settings(ctx, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if initial.CPUs != 2 || initial.MemoryMiB != 2048 {
		t.Fatalf("unexpected defaults: %+v", initial)
	}
	description, cpus, memory := "Work desktop", 4, 4096
	sharedPath := t.TempDir()
	sharedReadOnly := true
	updated, err := svc.Configure(ctx, "desktop", core.SettingsPatch{
		Description:    &description,
		CPUs:           &cpus,
		MemoryMiB:      &memory,
		SharedPath:     &sharedPath,
		SharedReadOnly: &sharedReadOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != description || updated.CPUs != cpus || updated.MemoryMiB != memory || updated.SharedPath != sharedPath || !updated.SharedReadOnly {
		t.Fatalf("settings were not persisted: %+v", updated)
	}
}

func TestClipboardAndTravelModeDefaultOnAndMachineOnly(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService()
	if _, err := svc.Create(ctx, core.Environment{Name: "desktop", Kind: core.Machine}); err != nil {
		t.Fatal(err)
	}
	initial, err := svc.Settings(ctx, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if initial.ClipboardDisabled || initial.TravelModeDisabled {
		t.Fatalf("expected clipboard sharing and travel mode on by default, got %+v", initial)
	}

	no := false
	updated, err := svc.Configure(ctx, "desktop", core.SettingsPatch{ShareClipboard: &no, TravelMode: &no})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.ClipboardDisabled || !updated.TravelModeDisabled {
		t.Fatalf("expected opt-out to persist, got %+v", updated)
	}

	if _, err := svc.Create(ctx, core.Environment{Name: "radic", Kind: core.Box}); err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := svc.Configure(ctx, "radic", core.SettingsPatch{ShareClipboard: &yes}); !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported for clipboard sharing on a Box, got %v", err)
	}
	if _, err := svc.Configure(ctx, "radic", core.SettingsPatch{TravelMode: &yes}); !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported for travel mode on a Box, got %v", err)
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

// fakeLinkBackend adds HostLinker on top of fakeBackend to test that
// Configure calls it best-effort when a Color patch is applied.
type fakeLinkBackend struct {
	*fakeBackend
	linkedColor map[string]string
	unlinked    map[string]bool
}

func newFakeLinkBackend(name string) *fakeLinkBackend {
	return &fakeLinkBackend{fakeBackend: newFakeBackend(name), linkedColor: map[string]string{}, unlinked: map[string]bool{}}
}

func (f *fakeLinkBackend) Link(ctx context.Context, env core.Environment, color string) (string, error) {
	f.linkedColor[env.Name] = color
	return "/fake/" + env.Name, nil
}

func (f *fakeLinkBackend) Unlink(ctx context.Context, env core.Environment) error {
	f.unlinked[env.Name] = true
	return nil
}

func TestConfigureColorValidatesPaletteAndLinksHost(t *testing.T) {
	ctx := context.Background()
	machine := newFakeLinkBackend("fake-machine")
	svc := core.NewService(&memStore{}, newFakeBackend("fake-box"), machine)
	if _, err := svc.Create(ctx, core.Environment{Name: "desktop", Kind: core.Machine}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	bogus := "chartreuse"
	if _, err := svc.Configure(ctx, "desktop", core.SettingsPatch{Color: &bogus}); err == nil {
		t.Fatal("expected an invalid color to be rejected")
	}

	blue := "blue"
	settings, err := svc.Configure(ctx, "desktop", core.SettingsPatch{Color: &blue})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if settings.Color != "blue" {
		t.Fatalf("expected color to be persisted, got %+v", settings)
	}
	if machine.linkedColor["desktop"] != "blue" {
		t.Fatalf("expected HostLinker.Link to be called with the new color, got %+v", machine.linkedColor)
	}

	if err := svc.Remove(ctx, "desktop"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !machine.unlinked["desktop"] {
		t.Fatal("expected HostLinker.Unlink to be called on Remove")
	}
}

func TestConfigureColorOnBackendWithoutHostLinkerStillPersists(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService() // plain fakeBackend: no HostLinker
	if _, err := svc.Create(ctx, core.Environment{Name: "radic", Kind: core.Box}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	green := "green"
	settings, err := svc.Configure(ctx, "radic", core.SettingsPatch{Color: &green})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if settings.Color != "green" {
		t.Fatalf("expected color to be persisted even without HostLinker, got %+v", settings)
	}
}

// fakeAppExporterBackend adds AppExporter on top of fakeBackend to test
// Service.ListApps/ExportApp/UnexportApp.
type fakeAppExporterBackend struct {
	*fakeBackend
	apps       []core.App
	exported   []string
	unexported []string
}

func newFakeAppExporterBackend(name string) *fakeAppExporterBackend {
	return &fakeAppExporterBackend{fakeBackend: newFakeBackend(name)}
}

func (f *fakeAppExporterBackend) ListApps(ctx context.Context, env core.Environment) ([]core.App, error) {
	return f.apps, nil
}
func (f *fakeAppExporterBackend) ExportApp(ctx context.Context, env core.Environment, id string) error {
	f.exported = append(f.exported, id)
	return nil
}
func (f *fakeAppExporterBackend) UnexportApp(ctx context.Context, env core.Environment, id string) error {
	f.unexported = append(f.unexported, id)
	return nil
}

func TestAppExportLifecycle(t *testing.T) {
	ctx := context.Background()
	box := newFakeAppExporterBackend("fake-distrobox")
	box.apps = []core.App{{ID: "/usr/share/applications/mpv.desktop", Name: "mpv"}}
	svc := core.NewService(&memStore{}, box, newFakeBackend("fake-machine"))
	if _, err := svc.Create(ctx, core.Environment{Name: "dev", Kind: core.Box}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	apps, err := svc.ListApps(ctx, "dev")
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "mpv" {
		t.Fatalf("unexpected apps: %+v", apps)
	}

	if err := svc.ExportApp(ctx, "dev", apps[0].ID); err != nil {
		t.Fatalf("ExportApp: %v", err)
	}
	if len(box.exported) != 1 || box.exported[0] != apps[0].ID {
		t.Fatalf("expected backend ExportApp to be called with %s, got %v", apps[0].ID, box.exported)
	}

	if err := svc.UnexportApp(ctx, "dev", apps[0].ID); err != nil {
		t.Fatalf("UnexportApp: %v", err)
	}
	if len(box.unexported) != 1 || box.unexported[0] != apps[0].ID {
		t.Fatalf("expected backend UnexportApp to be called with %s, got %v", apps[0].ID, box.unexported)
	}
}

func TestAppExportOnUnsupportedBackendFails(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService() // plain fakeBackend: no AppExporter
	if _, err := svc.Create(ctx, core.Environment{Name: "dev", Kind: core.Box}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.ListApps(ctx, "dev"); !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
	if err := svc.ExportApp(ctx, "dev", "id"); !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
	if err := svc.UnexportApp(ctx, "dev", "id"); !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestSnapshotOnUnsupportedBackendFails(t *testing.T) {
	ctx := context.Background()
	svc, box, _ := newTestService() // fakeBackend does not implement SnapshotManager
	_ = box
	if _, err := svc.Create(ctx, core.Environment{Name: "radic", Kind: core.Box}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.CreateSnapshot(ctx, "radic", "Before upgrade"); !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestSnapshotLifecycle(t *testing.T) {
	ctx := context.Background()
	machine := newFakeSnapshotBackend("fake-machine")
	svc := core.NewService(&memStore{}, newFakeBackend("fake-box"), machine)

	if _, err := svc.Create(ctx, core.Environment{Name: "desktop", Kind: core.Machine}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	snap, err := svc.CreateSnapshot(ctx, "desktop", "Before upgrade")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if snap.Label != "Before upgrade" || snap.ID == "" {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	if !machine.snapshots[snap.ID] {
		t.Fatal("expected backend to record the snapshot tag")
	}

	list, err := svc.ListSnapshots(ctx, "desktop")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(list) != 1 || list[0].ID != snap.ID {
		t.Fatalf("unexpected snapshot list: %+v", list)
	}

	if err := svc.GoToSnapshot(ctx, "desktop", snap.ID); err != nil {
		t.Fatalf("GoToSnapshot: %v", err)
	}
	if err := svc.GoToSnapshot(ctx, "desktop", "does-not-exist"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown snapshot, got %v", err)
	}

	if err := svc.RemoveSnapshot(ctx, "desktop", snap.ID); err != nil {
		t.Fatalf("RemoveSnapshot: %v", err)
	}
	if machine.snapshots[snap.ID] {
		t.Fatal("expected backend RemoveSnapshot to be called")
	}
	list, err = svc.ListSnapshots(ctx, "desktop")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no snapshots after removal, got %+v", list)
	}
}

func TestSnapshotLimitDiscardsOldest(t *testing.T) {
	ctx := context.Background()
	machine := newFakeSnapshotBackend("fake-machine")
	svc := core.NewService(&memStore{}, newFakeBackend("fake-box"), machine)

	if _, err := svc.Create(ctx, core.Environment{Name: "desktop", Kind: core.Machine}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	limit := 2
	if _, err := svc.Configure(ctx, "desktop", core.SettingsPatch{SnapshotLimit: &limit}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	var ids []string
	for _, label := range []string{"one", "two", "three"} {
		snap, err := svc.CreateSnapshot(ctx, "desktop", label)
		if err != nil {
			t.Fatalf("CreateSnapshot(%s): %v", label, err)
		}
		ids = append(ids, snap.ID)
	}

	list, err := svc.ListSnapshots(ctx, "desktop")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected snapshot limit to cap history at 2, got %d: %+v", len(list), list)
	}
	if list[0].Label != "two" || list[1].Label != "three" {
		t.Fatalf("expected oldest snapshot discarded, got %+v", list)
	}
	if machine.snapshots[ids[0]] {
		t.Fatal("expected backend to have discarded the oldest snapshot too")
	}
}
