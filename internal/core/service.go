package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	env.Backend = backend.Name()

	if err := backend.Create(ctx, env); err != nil {
		return Environment{}, fmt.Errorf("create %s: %w", env.Name, err)
	}

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
