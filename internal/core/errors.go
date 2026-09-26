package core

import (
	"errors"
	"fmt"
)

// Sentinel errors for conditions callers (CLI, GUI) commonly need to
// branch on. Use errors.Is against these rather than string matching.
var (
	ErrNotFound      = errors.New("environment not found")
	ErrAlreadyExists = errors.New("environment already exists")
	ErrUnsupported   = errors.New("operation not supported by this backend")
)

// InvalidKindError is returned when a Kind string doesn't match a known
// EnvironmentKind.
type InvalidKindError struct {
	Value string
}

func (e *InvalidKindError) Error() string {
	return fmt.Sprintf("invalid environment kind %q: must be \"box\" or \"machine\"", e.Value)
}
