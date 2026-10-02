package core

import (
	"context"
	"fmt"
)

type progressKey struct{}

// WithProgress returns a context whose long operations (downloading a
// Box's image) report what they are doing to fn, one short stage at a
// time. Stages describe real steps; no backend invents a percentage it
// doesn't have.
func WithProgress(ctx context.Context, fn func(stage string)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

// ReportProgress tells the caller of ctx about a stage, if it asked to
// know (WithProgress).
func ReportProgress(ctx context.Context, format string, args ...any) {
	if fn, ok := ctx.Value(progressKey{}).(func(string)); ok && fn != nil {
		fn(fmt.Sprintf(format, args...))
	}
}
