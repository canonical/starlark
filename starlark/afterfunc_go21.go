//go:build go1.21
// +build go1.21

package starlark

import "context"

// afterFunc implements context.AfterFunc for releases before Go 1.21.
func afterFunc(ctx context.Context, f func()) (stop func() bool) {
	return context.AfterFunc(ctx, f)
}
