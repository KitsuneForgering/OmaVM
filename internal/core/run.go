package core

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// RunEphemeral runs a new Machine that leaves nothing behind: it is
// created, started without keeping changes, opened when open is true, and
// deleted once it shuts down (omavm run --ephemeral). There is no daemon:
// the caller waits here, checking its state every poll, until the guest
// powers off or ctx is cancelled. Cancelling ends the session at once
// (Force Stop): nothing of it was going to be kept anyway.
//
// Ephemeral Boxes belong to the Disposable mode on direct containers
// (Backend Rules) and aren't offered here.
func (s *Service) RunEphemeral(ctx context.Context, env Environment, open bool, poll time.Duration) (Environment, error) {
	if env.Kind != Machine {
		return Environment{}, Unsupportedf("only Desktops (Machines) can run ephemeral for now; disposable Boxes are not available yet")
	}
	if env.Name == "" {
		id, err := newID()
		if err != nil {
			return Environment{}, err
		}
		env.Name = "ephemeral-" + id[:6]
	}
	// Here and gone: never listed in the app launcher.
	env.Settings.LauncherDisabled = true
	created, err := s.Create(ctx, env)
	if err != nil {
		return Environment{}, err
	}
	name := created.Name
	defer func() {
		// The caller's context may be cancelled by now.
		cleanup := context.Background()
		if status, err := s.Status(cleanup, name); err == nil && status.State != StateStopped {
			if err := s.ForceStop(cleanup, name); err != nil {
				slog.Warn("ephemeral environment could not be stopped", "environment", name, "error", err)
			}
		}
		if err := s.Remove(cleanup, name); err != nil {
			slog.Error("ephemeral environment left behind", "environment", name, "error", err)
		}
	}()

	if err := s.StartEphemeral(ctx, name); err != nil {
		return created, err
	}
	if open {
		if err := s.Open(ctx, name); err != nil {
			return created, err
		}
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return created, fmt.Errorf("ended %s: %w", name, ctx.Err())
		case <-ticker.C:
		}
		status, err := s.Status(ctx, name)
		if err != nil {
			continue // a slow answer is not the end of the session
		}
		if status.State == StateStopped {
			return created, nil
		}
	}
}
