package starlark_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canonical/starlark/starlark"
)

func TestAfterFuncCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := make(chan struct{})
	stop := starlark.AfterFunc(ctx, func() {
		close(c)
	})
	if stop() != false {
		t.Errorf("function should run immediately, stop should have no effect")
	}
	<-c
}

func TestAfterFuncUncancelableContext(t *testing.T) {
	starlark.AfterFunc(context.Background(), func() {
		t.Fatal("should never run")
	})
	time.Sleep(time.Millisecond * 200)
}

func TestAfterFuncNormalOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var run atomic.Bool
	done := make(chan struct{})
	starlark.AfterFunc(ctx, func() {
		run.Store(true)
		close(done)
	})
	time.Sleep(time.Millisecond * 200)
	if run.Load() == true {
		t.Errorf("function ran prior to context cancelation")
	}
	cancel()
	<-done
	if run.Load() == false {
		t.Errorf("function did not run on context cancelation")
	}
}

func TestAfterFuncStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := starlark.AfterFunc(ctx, func() {
		t.Fatal("should never run")
	})
	if stop() == false {
		t.Errorf("stop should return true when function did not run")
	}
}

func TestAfterFuncStopAlreadyRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	stop := starlark.AfterFunc(ctx, func() {
		close(done)
	})
	cancel()
	<-done
	if stop() == true {
		t.Errorf("stop should return false when function already run")
	}
}
