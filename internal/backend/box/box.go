// Package box routes new Development Boxes to Distrobox while preserving
// lifecycle access to environments created by the former direct engine.
package box

import (
	"context"
	"fmt"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

type Backend struct {
	primary core.Backend
	legacy  core.Backend
}

func New(primary, legacy core.Backend) *Backend { return &Backend{primary: primary, legacy: legacy} }
func (b *Backend) Name() string                 { return b.primary.Name() }

func (b *Backend) forEnv(env core.Environment) core.Backend {
	if env.Backend == "podman" || env.Backend == "docker" {
		return b.legacy
	}
	return b.primary
}

func (b *Backend) Create(ctx context.Context, env core.Environment) error {
	return b.primary.Create(ctx, env)
}
func (b *Backend) Start(ctx context.Context, env core.Environment) error {
	return b.forEnv(env).Start(ctx, env)
}
func (b *Backend) Open(ctx context.Context, env core.Environment) error {
	return b.forEnv(env).Open(ctx, env)
}
func (b *Backend) Stop(ctx context.Context, env core.Environment) error {
	return b.forEnv(env).Stop(ctx, env)
}
func (b *Backend) Status(ctx context.Context, env core.Environment) (core.Status, error) {
	return b.forEnv(env).Status(ctx, env)
}
func (b *Backend) Exec(ctx context.Context, env core.Environment, args []string) error {
	return b.forEnv(env).Exec(ctx, env, args)
}
func (b *Backend) Remove(ctx context.Context, env core.Environment) error {
	return b.forEnv(env).Remove(ctx, env)
}

// appExporter resolves env's backend as an AppExporter, or ErrUnsupported
// when it's routed to the legacy engine — distrobox-export (the Blend
// Mode base) is a Distrobox-native feature the legacy container adapter
// never had.
func (b *Backend) appExporter(env core.Environment) (core.AppExporter, error) {
	exporter, ok := b.forEnv(env).(core.AppExporter)
	if !ok {
		return nil, fmt.Errorf("%w: legacy container Boxes don't support application export", core.ErrUnsupported)
	}
	return exporter, nil
}

func (b *Backend) ListApps(ctx context.Context, env core.Environment) ([]core.App, error) {
	exporter, err := b.appExporter(env)
	if err != nil {
		return nil, err
	}
	return exporter.ListApps(ctx, env)
}

func (b *Backend) ExportApp(ctx context.Context, env core.Environment, id string) error {
	exporter, err := b.appExporter(env)
	if err != nil {
		return err
	}
	return exporter.ExportApp(ctx, env, id)
}

func (b *Backend) UnexportApp(ctx context.Context, env core.Environment, id string) error {
	exporter, err := b.appExporter(env)
	if err != nil {
		return err
	}
	return exporter.UnexportApp(ctx, env, id)
}
