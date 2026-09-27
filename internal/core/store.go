package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Store persists the set of known environments. There is no omavmd yet,
// so each CLI invocation is a fresh process: state must round-trip
// through disk rather than live in memory.
type Store interface {
	Lock(context.Context) (func(), error)
	Load() ([]Environment, error)
	Save([]Environment) error
}

// Lock serializes read-modify-write operations across CLI processes.
// ponytail: one registry lock; use per-environment locks if contention matters.
func (s *FileStore) Lock(ctx context.Context) (func(), error) {
	f, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// FileStore is a Store backed by a single JSON file.
type FileStore struct {
	Path string
}

// NewFileStore returns a FileStore rooted at the default OmaVM state
// directory ($XDG_STATE_HOME/omavm, falling back to ~/.local/state/omavm),
// creating it if necessary.
func NewFileStore() (*FileStore, error) {
	dir, err := StateDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	return &FileStore{Path: filepath.Join(dir, "environments.json")}, nil
}

// StateDir returns the directory OmaVM stores its local state under.
func StateDir() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "omavm"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "omavm"), nil
}

func (s *FileStore) Load() ([]Environment, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}
	var envs []Environment
	if err := json.Unmarshal(data, &envs); err != nil {
		return nil, fmt.Errorf("parse state file %s: %w", s.Path, err)
	}
	return envs, nil
}

func (s *FileStore) Save(envs []Environment) error {
	data, err := json.MarshalIndent(envs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return fmt.Errorf("commit state file: %w", err)
	}
	return nil
}
