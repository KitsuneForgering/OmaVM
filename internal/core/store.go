package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Store persists the set of known environments. There is no omavmd yet,
// so each CLI invocation is a fresh process: state must round-trip
// through disk rather than live in memory.
type Store interface {
	// Lock serializes read-modify-write cycles of the registry across
	// processes. It is held only while the registry is read and written,
	// never across a backend call.
	Lock(context.Context) (func(), error)
	// LockEnvironment serializes the operations that change one
	// environment (start, stop, snapshots, removal, ...), held across the
	// backend call, so they wait their turn instead of racing each other
	// in the engine while other environments stay free. Always taken
	// before Lock, never while holding it.
	LockEnvironment(ctx context.Context, id string) (func(), error)
	// TryLockEnvironment takes the environment's lock only if it is free
	// right now. A lock dies with the process holding it, so a free lock
	// on an environment marked as being created or removed means that
	// operation was interrupted.
	TryLockEnvironment(id string) (unlock func(), ok bool, err error)
	Load() ([]Environment, error)
	Save([]Environment) error
}

func (s *FileStore) Lock(ctx context.Context) (func(), error) {
	return lockFile(ctx, s.Path+".lock", true, s.OnWait)
}

func (s *FileStore) LockEnvironment(ctx context.Context, id string) (func(), error) {
	path, err := s.environmentLockPath(id)
	if err != nil {
		return nil, err
	}
	return lockFile(ctx, path, true, s.OnWait)
}

func (s *FileStore) TryLockEnvironment(id string) (func(), bool, error) {
	path, err := s.environmentLockPath(id)
	if err != nil {
		return nil, false, err
	}
	unlock, err := lockFile(context.Background(), path, false, nil)
	if errors.Is(err, errLocked) {
		return nil, false, nil
	}
	return unlock, err == nil, err
}

// environmentLockPath is locks/<id>.lock next to the registry. The files
// stay after an environment is removed: deleting a lock file others may
// be waiting on would let two processes hold "the same" lock.
func (s *FileStore) environmentLockPath(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return "", fmt.Errorf("invalid environment id %q", id)
	}
	dir := filepath.Join(filepath.Dir(s.Path), "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create lock dir: %w", err)
	}
	return filepath.Join(dir, id+".lock"), nil
}

var errLocked = errors.New("locked")

// lockFile takes an exclusive flock on path, waiting for it when wait is
// true (calling onWait once if it has to) or failing with errLocked.
func lockFile(ctx context.Context, path string, wait bool, onWait func()) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	waited := false
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			f.Close()
			return nil, err
		}
		if !wait {
			f.Close()
			return nil, errLocked
		}
		if err := ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		if !waited && onWait != nil {
			onWait()
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

// registryVersion is the newest format of environments.json this OmaVM
// reads. Format 1 is the bare list of environments, which is still what
// Save writes, so an older OmaVM can read the registry after a downgrade.
// When a change can't be expressed by a new field whose zero value keeps
// the old behavior, bump this, write registryFile from Save and convert
// older formats in Load: every OmaVM since this one refuses a newer format
// with a clear message instead of rewriting it and losing what it added.
const registryVersion = 1

// registryFile is environments.json from format 2 on.
type registryFile struct {
	Version      int           `json:"version"`
	Environments []Environment `json:"environments"`
}

func (s *FileStore) Load() ([]Environment, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}
	damaged := func(err error) error {
		return fmt.Errorf("the environment registry %s is damaged (%v); OmaVM keeps the previous version at %s.bak — copy it over the damaged file to recover", s.Path, err, s.Path)
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		var envs []Environment
		if err := json.Unmarshal(data, &envs); err != nil {
			return nil, damaged(err)
		}
		return envs, nil
	}
	var file registryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, damaged(err)
	}
	if file.Version > registryVersion {
		// Rewriting it would drop whatever the newer format added.
		return nil, fmt.Errorf("the environment registry %s was written by a newer OmaVM (format %d, this one reads up to %d); update OmaVM", s.Path, file.Version, registryVersion)
	}
	return file.Environments, nil
}

// Save replaces the registry atomically and durably: the new content is
// synced before it replaces the old file, so a crash or power loss leaves
// either the old or the new registry, never a truncated one. The previous
// version stays at <path>.bak for recovery.
func (s *FileStore) Save(envs []Environment) error {
	if envs == nil {
		envs = []Environment{}
	}
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
	// Create the replacement backup before touching the existing one. A
	// failed link must leave the last good backup available for recovery.
	backup := s.Path + ".bak"
	if current, err := os.ReadFile(s.Path); err == nil && json.Valid(current) {
		pending := backup + ".tmp"
		if err := os.Remove(pending); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("prepare registry backup: %w", err)
		}
		if err := os.Link(s.Path, pending); err != nil {
			return fmt.Errorf("create registry backup: %w", err)
		}
		if err := os.Rename(pending, backup); err != nil {
			return fmt.Errorf("replace registry backup: %w", err)
		}
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
