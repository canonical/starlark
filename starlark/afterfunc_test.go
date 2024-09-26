package starlark_test

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/starlark/starlark"
)

func TestAfterFuncCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	stop := starlark.AfterFunc(ctx, func() {
		close(done)
	})
	if stop() {
		t.Errorf("function should run immediately, stop should have no effect")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Errorf("channel was not closed within reasonable time")
	}
}

func TestAfterFuncUncancelableContext(t *testing.T) {
	starlark.AfterFunc(context.Background(), func() {
		t.Fatal("function unexpectedly called")
	})
	time.Sleep(time.Millisecond * 200)
}

func TestAfterFuncNormalOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	starlark.AfterFunc(ctx, func() {
		close(done)
	})
	select {
	case <-done:
		t.Errorf("after func ran before context was cancelled")
	case <-time.After(time.Millisecond * 200):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Errorf("after func not called within reasonable time")
	}
}

func TestAfterFuncStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := starlark.AfterFunc(ctx, func() {
		t.Fatal("should never run")
	})
	if !stop() {
		t.Errorf("unexpected stop result: false")
	}
}

func TestAfterFuncStopAlreadyRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	stop := starlark.AfterFunc(ctx, func() {
		close(done)
	})
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Errorf("after func not called within reasonable time")
	}
	if stop() {
		t.Errorf("unexpected stop result: true")
	}
}
