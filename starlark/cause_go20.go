//go:build go1.20
// +build go1.20

package starlark

import "context"

func cause(ctx context.Context) error {
	return context.Cause(ctx)
}
