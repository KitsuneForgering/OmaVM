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
	ErrInvalidInput  = errors.New("invalid input")
	// ErrBusy means another operation on the environment (its creation
	// or removal) is still running.
	ErrBusy = errors.New("environment is busy")
)

// Invalidf and Unsupportedf build errors that match ErrInvalidInput and
// ErrUnsupported with errors.Is but read as just their message: the
// sentinel is for callers to branch on, and "operation not supported by
// this backend" would show people an infrastructure term the UI avoids.
func Invalidf(format string, args ...any) error {
	return &classifiedError{kind: ErrInvalidInput, err: fmt.Errorf(format, args...)}
}

func Unsupportedf(format string, args ...any) error {
	return &classifiedError{kind: ErrUnsupported, err: fmt.Errorf(format, args...)}
}

func Busyf(format string, args ...any) error {
	return &classifiedError{kind: ErrBusy, err: fmt.Errorf(format, args...)}
}

type classifiedError struct {
	kind error
	err  error
}

func (e *classifiedError) Error() string   { return e.err.Error() }
func (e *classifiedError) Unwrap() []error { return []error{e.kind, e.err} }

// InvalidKindError is returned when a Kind string doesn't match a known
// EnvironmentKind.
type InvalidKindError struct {
	Value string
}

func (e *InvalidKindError) Error() string {
	return fmt.Sprintf("invalid environment kind %q: must be \"box\" or \"machine\"", e.Value)
}
