//go:build go1.20
// +build go1.20

package starlark

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/canonical/starlark/syntax"
)

func TestContextCancelCause(t *testing.T) {
	expectedCause := errors.New("time is running out")

	parentCtx, cancel := context.WithCancelCause(context.Background())
	thread := &Thread{}
	thread.SetParentContext(parentCtx)
	threadCtx := thread.Context()

	cancel(expectedCause)

	select {
	case <-threadCtx.Done():
	case <-time.After(time.Second):
		t.Fatalf("thread was not cancelled within reasonable time")
	}

	if actualCause := context.Cause(threadCtx); actualCause != expectedCause {
		t.Errorf("expected %v, got %v", expectedCause, actualCause)
	}

	_, err := ExecFileOptions(&syntax.FileOptions{}, thread, "test.star", "pass", nil)
	if err == nil {
		t.Errorf("expected error")
	} else if !errors.Is(err, expectedCause) {
		t.Errorf("cause not recorded: %v", err)
	}
}
