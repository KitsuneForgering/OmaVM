package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Service is the OmaVM Core: the single entry point the CLI and, later,
// the GUI call into. It owns environment lifecycle and persistence; it
// never runs backend-specific commands itself, only through Backend.
type Service struct {
	store    Store
	backends map[EnvironmentKind]Backend
}

// NewService wires a Store with the backends responsible for each Kind.
// A nil backend for a given Kind is valid: operations on that Kind will
// fail with ErrUnsupported instead of panicking.
func NewService(store Store, box, machine Backend) *Service {
	return &Service{
		store: store,
		backends: map[EnvironmentKind]Backend{
			Box:     box,
			Machine: machine,
		},
	}
}

func (s *Service) backendFor(kind EnvironmentKind) (Backend, error) {
	b := s.backends[kind]
	if b == nil {
		return nil, fmt.Errorf("%w: no backend registered for kind %s", ErrUnsupported, kind)
	}
	return b, nil
}

func findEnvironment(envs []Environment, name string) (Environment, int) {
	for i, e := range envs {
		if e.Name == name {
			return e, i
		}
	}
	return Environment{}, -1
}

// List returns every known environment, regardless of Kind or Backend.
func (s *Service) List(ctx context.Context) ([]Environment, error) {
	return s.store.Load()
}

// Create registers a new environment and delegates its creation to the
// Backend responsible for its Kind.
func (s *Service) Create(ctx context.Context, env Environment) (Environment, error) {
	envs, err := s.store.Load()
	if err != nil {
		return Environment{}, err
	}
	if _, idx := findEnvironment(envs, env.Name); idx != -1 {
		return Environment{}, fmt.Errorf("%w: %s", ErrAlreadyExists, env.Name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return Environment{}, err
	}

	id, err := newID()
	if err != nil {
		return Environment{}, err
	}
	env.ID = id

	if err := backend.Create(ctx, env); err != nil {
		return Environment{}, fmt.Errorf("create %s: %w", env.Name, err)
	}
	// Persist ownership only after the backend completed creation.
	env.Backend = backend.Name()

	envs = append(envs, env)
	if err := s.store.Save(envs); err != nil {
		return Environment{}, err
	}
	return env, nil
}

func (s *Service) resolve(ctx context.Context, name string) (Environment, Backend, error) {
	envs, err := s.store.Load()
	if err != nil {
		return Environment{}, nil, err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return Environment{}, nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return Environment{}, nil, err
	}
	return env, backend, nil
}

func (s *Service) Start(ctx context.Context, name string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if err := backend.Start(ctx, env); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	return nil
}

func (s *Service) Open(ctx context.Context, name string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if err := backend.Open(ctx, env); err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	return nil
}

// Preview returns a screenshot path for environments whose Backend
// implements Previewer (currently only Machines). ErrUnsupported for
// anything else — a Box has no display, and that's never hidden behind
// a placeholder image pretending otherwise.
func (s *Service) Preview(ctx context.Context, name string) (string, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return "", err
	}
	previewer, ok := backend.(Previewer)
	if !ok {
		return "", fmt.Errorf("%w: %s has no preview capability", ErrUnsupported, name)
	}
	path, err := previewer.Preview(ctx, env)
	if err != nil {
		return "", fmt.Errorf("preview %s: %w", name, err)
	}
	return path, nil
}

func (s *Service) Stop(ctx context.Context, name string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if err := backend.Stop(ctx, env); err != nil {
		return fmt.Errorf("stop %s: %w", name, err)
	}
	return nil
}

func (s *Service) advancedLifecycle(ctx context.Context, name string) (Environment, AdvancedLifecycle, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return Environment{}, nil, err
	}
	lifecycle, ok := backend.(AdvancedLifecycle)
	if !ok {
		return Environment{}, nil, fmt.Errorf("%w: %s does not support advanced lifecycle", ErrUnsupported, name)
	}
	return env, lifecycle, nil
}

func (s *Service) Restart(ctx context.Context, name string) error {
	env, lifecycle, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	return lifecycle.Restart(ctx, env)
}

func (s *Service) Pause(ctx context.Context, name string) error {
	env, lifecycle, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	return lifecycle.Pause(ctx, env)
}

func (s *Service) Resume(ctx context.Context, name string) error {
	env, lifecycle, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	return lifecycle.Resume(ctx, env)
}

func (s *Service) ForceStop(ctx context.Context, name string) error {
	env, lifecycle, err := s.advancedLifecycle(ctx, name)
	if err != nil {
		return err
	}
	return lifecycle.ForceStop(ctx, env)
}

func (s *Service) Status(ctx context.Context, name string) (Status, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return Status{}, err
	}
	status, err := backend.Status(ctx, env)
	if err != nil {
		return Status{}, fmt.Errorf("status %s: %w", name, err)
	}
	return status, nil
}

func (s *Service) Integration(ctx context.Context, name string) (IntegrationReport, error) {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return IntegrationReport{}, err
	}
	reporter, ok := backend.(IntegrationReporter)
	if !ok {
		return IntegrationReport{}, fmt.Errorf("%w: %s has no guest integration report", ErrUnsupported, name)
	}
	return reporter.Integration(ctx, env)
}

func (s *Service) Settings(ctx context.Context, name string) (EnvironmentSettings, error) {
	env, _, err := s.resolve(ctx, name)
	if err != nil {
		return EnvironmentSettings{}, err
	}
	return env.EffectiveSettings(), nil
}

func (s *Service) Configure(ctx context.Context, name string, patch SettingsPatch) (EnvironmentSettings, error) {
	envs, err := s.store.Load()
	if err != nil {
		return EnvironmentSettings{}, err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return EnvironmentSettings{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	settings := env.EffectiveSettings()
	if patch.Description != nil {
		settings.Description = strings.TrimSpace(*patch.Description)
		if len(settings.Description) > 500 {
			return EnvironmentSettings{}, fmt.Errorf("description must be at most 500 characters")
		}
	}
	if patch.CPUs != nil || patch.MemoryMiB != nil {
		if env.Kind != Machine {
			return EnvironmentSettings{}, fmt.Errorf("%w: CPU and memory settings only apply to Machines", ErrUnsupported)
		}
	}
	if patch.SharedPath != nil || patch.SharedReadOnly != nil {
		if env.Kind != Machine {
			return EnvironmentSettings{}, fmt.Errorf("%w: shared folders only apply to Machines", ErrUnsupported)
		}
	}
	if patch.SharedPath != nil {
		path := strings.TrimSpace(*patch.SharedPath)
		if path != "" {
			path, err = filepath.Abs(path)
			if err != nil {
				return EnvironmentSettings{}, fmt.Errorf("resolve shared folder: %w", err)
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				return EnvironmentSettings{}, fmt.Errorf("shared folder: %w", statErr)
			}
			if !info.IsDir() {
				return EnvironmentSettings{}, fmt.Errorf("shared folder must be a directory")
			}
		}
		settings.SharedPath = path
	}
	if patch.SharedReadOnly != nil {
		settings.SharedReadOnly = *patch.SharedReadOnly
	}
	if patch.CPUs != nil {
		if *patch.CPUs < 1 || *patch.CPUs > 64 {
			return EnvironmentSettings{}, fmt.Errorf("cpus must be between 1 and 64")
		}
		settings.CPUs = *patch.CPUs
	}
	if patch.MemoryMiB != nil {
		if *patch.MemoryMiB < 256 || *patch.MemoryMiB > 262144 {
			return EnvironmentSettings{}, fmt.Errorf("memory-mib must be between 256 and 262144")
		}
		settings.MemoryMiB = *patch.MemoryMiB
	}
	env.Settings = settings
	envs[idx] = env
	if err := s.store.Save(envs); err != nil {
		return EnvironmentSettings{}, err
	}
	return settings, nil
}

func (s *Service) Exec(ctx context.Context, name string, args []string) error {
	env, backend, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	if err := backend.Exec(ctx, env, args); err != nil {
		return fmt.Errorf("exec %s: %w", name, err)
	}
	return nil
}

// Remove deletes an environment through its Backend and drops it from
// the store. It only forgets the environment locally if the backend
// confirms removal, so a failed backend removal never leaves state
// pointing at nothing.
func (s *Service) Remove(ctx context.Context, name string) error {
	envs, err := s.store.Load()
	if err != nil {
		return err
	}
	env, idx := findEnvironment(envs, name)
	if idx == -1 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	backend, err := s.backendFor(env.Kind)
	if err != nil {
		return err
	}
	if err := backend.Remove(ctx, env); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	envs = append(envs[:idx], envs[idx+1:]...)
	return s.store.Save(envs)
}

func newID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
