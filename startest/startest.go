package startest

import (
	"math"
	"runtime"
	"testing"

	"github.com/canonical/starlark/starlark"
	"gopkg.in/check.v1"
)

type testBase interface {
	Error(err ...interface{})
	Errorf(format string, err ...interface{})
	Failed() bool
}

type allocTest struct {
	predefined  starlark.StringDict
	maxAllocs   uint64
	expectedErr string
	testBase
}

var _ testBase = &testing.T{}
var _ testBase = &testing.B{}
var _ testBase = &check.C{}

func From(base testBase) *allocTest {
	return &allocTest{testBase: base}
}

func (test *allocTest) AddBuiltin(fn *starlark.Builtin) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[fn.Name()] = fn
}

func (test *allocTest) AddValue(name string, value starlark.Value) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[name] = value
}

func (test *allocTest) SetMaxAllocs(maxAllocs uint64) {
	test.maxAllocs = maxAllocs
}

func (test *allocTest) Expect(err string) {
	test.expectedErr = err
}

func (test *allocTest) RunBuiltin(fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) {
	test.RunThread(func(th *starlark.Thread, globals starlark.StringDict) interface{} {
		result, err := starlark.Call(th, fn, args, kwargs)
		if err != nil {
			test.Error(err)
		}

		return result
	})
}

func (test *allocTest) RunThread(fn func(*starlark.Thread, starlark.StringDict) interface{}) {
	thread := &starlark.Thread{}
	_, measured := measureMemory(func() interface{} {
		return fn(thread, test.predefined)
	})

	if test.maxAllocs != 0 {
		if measured > test.maxAllocs {
			test.Errorf("too many measured allocations")
		}
		if thread.Allocs() > test.maxAllocs {
			test.Errorf("too many declared allocations")
		}
	}
}

func measureMemory(generate func() interface{}) (interface{}, uint64) {
	measured := uint64(math.MaxUint64)
	var result interface{}

	for i := 0; i < 20; i++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)

		iterationResult := generate()

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&after)

		iterationMeasure := uint64(after.Alloc - before.Alloc)

		if iterationMeasure == measured {
			break
		} else if iterationMeasure < measured {
			measured = iterationMeasure
			result = iterationResult
		}
	}

	return result, measured
}

func (test *allocTest) N() int {
	if b, ok := test.testBase.(*testing.B); ok {
		return b.N
	}
	return 1
}

func (test *allocTest) run(fn func() error) {
	test.Errorf("Not implemented")
}
