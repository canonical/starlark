//go:build !go1.21
// +build !go1.21

package starlark

import (
	"context"
	"sync/atomic"
)

// afterFunc implements context.AfterFunc for releases before Go 1.21.
func afterFunc(ctx context.Context, f func()) (stop func() bool) {
	if ctx.Err() != nil {
		go f()
		return func() bool { return false }
	}

	var run uint32
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if atomic.CompareAndSwapUint32(&run, 0, 1) {
				close(done)
				f()
			}
		case <-done:
		}
	}()

	return func() bool {
		if atomic.CompareAndSwapUint32(&run, 0, 1) {
			close(done)
			return true
		}
		return false
	}
}
