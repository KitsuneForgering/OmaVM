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
	waited := false
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
		if !waited && s.OnWait != nil {
			s.OnWait()
		}
		waited = true
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
	// OnWait, when set, is called once if Lock has to wait for another
	// OmaVM process (a Box pulling its image holds the registry for
	// minutes), so a caller can say why nothing is happening yet.
	OnWait func()
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
		return nil, fmt.Errorf("the environment registry %s is damaged (%v); OmaVM keeps the previous version at %s.bak — copy it over the damaged file to recover", s.Path, err, s.Path)
	}
	return envs, nil
}

// Save replaces the registry atomically and durably: the new content is
// synced before it replaces the old file, so a crash or power loss leaves
// either the old or the new registry, never a truncated one. The previous
// version stays at <path>.bak for recovery.
func (s *FileStore) Save(envs []Environment) error {
	data, err := json.MarshalIndent(envs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp := s.Path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write state file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync state file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	// A hard link keeps the old registry reachable as .bak without a
	// moment where the registry itself is missing (readers don't lock).
	// Only a registry that still parses is kept: never replace a good
	// backup with a damaged file.
	backup := s.Path + ".bak"
	if current, err := os.ReadFile(s.Path); err == nil && json.Valid(current) {
		_ = os.Remove(backup)
		_ = os.Link(s.Path, backup)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return fmt.Errorf("commit state file: %w", err)
	}
	if dir, err := os.Open(filepath.Dir(s.Path)); err == nil {
		_ = dir.Sync()
		dir.Close()
	}
	return nil
}
