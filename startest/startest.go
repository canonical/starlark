package startest

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/canonical/starlark/starlark"
	"gopkg.in/check.v1"
)

type testBase interface {
	Error(err ...interface{})
	Errorf(format string, err ...interface{})
	Failed() bool
}

type starTest struct {
	predefined  starlark.StringDict
	maxAllocs   uint64
	expectedErr string
	tracked     []interface{}
	N           int
	testBase
}

var _ testBase = &testing.T{}
var _ testBase = &testing.B{}
var _ testBase = &check.C{}

func From(base testBase) *starTest {
	return &starTest{testBase: base}
}

func (test *starTest) AddBuiltin(fn *starlark.Builtin) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[fn.Name()] = fn
}

func (test *starTest) AddValue(name string, value starlark.Value) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[name] = value
}

func (test *starTest) SetMaxAllocs(maxAllocs uint64) {
	test.maxAllocs = maxAllocs
}

func (test *starTest) Expect(err string) {
	test.expectedErr = err
}

func (test *starTest) RunBuiltin(fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) {
	test.RunThread(func(th *starlark.Thread, globals starlark.StringDict) {
		for i := 0; i < test.N; i++ {
			result, err := starlark.Call(th, fn, args, kwargs)
			if err != nil {
				test.Error(err)
			}

			test.Track(result)
		}
	})
}

func (test *starTest) RunThread(fn func(*starlark.Thread, starlark.StringDict)) {
	thread := &starlark.Thread{}
	measured := test.measureMemory(func() {
		fn(thread, test.predefined)
	})

	if !test.Failed() && test.maxAllocs != 0 {
		if measured > test.maxAllocs {
			test.Errorf("too many measured allocations")
		}
		if thread.Allocs() > test.maxAllocs {
			test.Errorf("too many declared allocations")
		}
	}
}

func (test *starTest) Track(v ...interface{}) {
	test.tracked = append(test.tracked, v...)
}

func (test *starTest) measureMemory(fn func()) uint64 {
	defer func() { test.tracked = make([]interface{}, 0) }()

	startNano := time.Now().Nanosecond()

	const nMax = 100_000
	const memoryTarget = 100 * 2 << 20
	const timeMax = 1000

	var memoryUsed int64
	var valueTrackerOverhead int64
	test.N = 0
	for n := int64(1); !test.Failed() && memoryUsed-valueTrackerOverhead < memoryTarget && n < nMax && (time.Now().Nanosecond()-startNano) < timeMax; {
		valueTrackerOverhead = int64(cap(test.tracked)) * int64(unsafe.Sizeof(interface{}(nil)))
		last := n
		prevIters := int64(test.N)
		prevMemory := memoryUsed
		if prevMemory <= 0 {
			prevMemory = 1
		}
		n = memoryTarget * prevIters / prevMemory
		n += n / 5
		maxGrowth := last * 100
		minGrowth := last + 1
		if n > maxGrowth {
			n = maxGrowth
		} else if n < minGrowth {
			n = minGrowth
		}

		if n > nMax {
			n = nMax
		}

		test.N = int(n)

		var before, after runtime.MemStats
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)

		fn()

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&after)

		iterationMeasure := int64(after.Alloc - before.Alloc)
		if iterationMeasure <= 0 {
			memoryUsed++
		} else {
			memoryUsed += iterationMeasure
		}
	}

	if test.Failed() {
		return 0
	}

	return uint64(float64(memoryUsed-valueTrackerOverhead) / float64(test.N))
}

func (test *starTest) run(fn func() error) {
	test.Errorf("Not implemented")
}
