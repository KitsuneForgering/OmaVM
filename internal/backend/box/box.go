// Package box routes new Development Boxes to Distrobox while preserving
// lifecycle access to environments created by the former direct engine.
package box

import (
	"context"

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
