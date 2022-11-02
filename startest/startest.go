package startest

import (
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
	err         error
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
	_, measured := MeasureMemory(func() interface{} {
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

func MeasureMemory(generate func() interface{}) (interface{}, uint64) {
	var result interface{}
	const maxVotes = 21
	const winningMargin = 4

	measurements := make(map[uint64]int, maxVotes)
	for i := 0; i < maxVotes; i++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)

		result = generate()

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&after)

		measured := after.Alloc - before.Alloc
		measurements[measured]++

		if i >= winningMargin {
			if measurement, margin := mostPopular(measurements); margin >= winningMargin {
				return result, measurement
			}
		}
	}

	measurement, _ := mostPopular(measurements)
	return result, measurement
}

func mostPopular(m map[uint64]int) (winner uint64, margin int) {
	var winnerVotes, runnerUpVotes int

	for m, v := range m {
		if winnerVotes < v {
			runnerUpVotes = winnerVotes
			winner = m
			winnerVotes = v
		} else if runnerUpVotes < v {
			runnerUpVotes = v
		}
	}

	return winner, winnerVotes - runnerUpVotes
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
