package startest

import (
	"math"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/canonical/starlark/starlark"
	"gopkg.in/check.v1"
)

type TestBase interface {
	Error(err ...interface{})
	Errorf(format string, err ...interface{})
	Failed() bool
	Log(args ...interface{})
	Logf(fmt string, args ...interface{})
}

type S struct {
	maxAllocs uint64
	tracked   []interface{}
	N         int
	TestBase
}

var _ TestBase = &testing.T{}
var _ TestBase = &testing.B{}
var _ TestBase = &check.C{}

// From returns a new starTest instance with a given test base.
func From(base testBase) *S {
	return &S{TestBase: base, maxAllocs: math.MaxUint64}
}

// SetMaxAllocs optionally sets the max allocations allowed per test.N
func (test *S) SetMaxAllocs(maxAllocs uint64) {
	test.maxAllocs = maxAllocs
}

// RunThread tests a function which has access to a starlark thread and a global environment
func (test *S) RunThread(fn func(*starlark.Thread)) {
	thread := &starlark.Thread{}

	memorySum, nSum := test.measureMemory(func() {
		fn(thread)
	})

	if test.Failed() {
		return
	}

	meanMeasured := memorySum / nSum

	if meanMeasured > test.maxAllocs {
		test.Errorf("measured memory is above maximum (%d > %d)", meanMeasured, test.maxAllocs)
	}

	if meanDeclared := thread.Allocs() / nSum; meanDeclared > test.maxAllocs {
		test.Errorf("declared allocations are above maximum (%d > %d)", meanDeclared, test.maxAllocs)
	}

	if test.maxAllocs != math.MaxUint64 {
		if meanMeasured > thread.Allocs() {
			test.Errorf("measured memory is above declared allocations (%d > %d)", meanMeasured, thread.Allocs())
		}
	}
}

// Track causes the memory of the passed objects to be measured
func (test *S) Track(values ...interface{}) {
	test.tracked = append(test.tracked, values...)
}

func (test *S) measureMemory(fn func()) (memorySum, nSum uint64) {
	startNano := time.Now().Nanosecond()

	const nMax = 100_000
	const memoryMax = 100 * 2 << 20
	const timeMax = 1e9

	var memoryUsed uint64
	var valueTrackerOverhead uint64
	test.N = 0
	nSum = 0

	for n := uint64(0); !test.Failed() && memoryUsed-valueTrackerOverhead < memoryMax && n < nMax && (time.Now().Nanosecond()-startNano) < timeMax; {
		last := n
		prevIters := uint64(test.N)
		prevMemory := memoryUsed
		if prevMemory <= 0 {
			prevMemory = 1
		}
		n = memoryMax * prevIters / prevMemory
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
		nSum += n

		var before, after runtime.MemStats
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)

		fn()

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&after)

		iterationMeasure := int64(after.Alloc - before.Alloc)
		valueTrackerOverhead += uint64(cap(test.tracked)) * uint64(unsafe.Sizeof(interface{}(nil)))
		test.tracked = nil
		if iterationMeasure > 0 {
			memoryUsed += uint64(iterationMeasure)
		}
	}

	if test.Failed() {
		return 0, 1
	}

	if valueTrackerOverhead > memoryUsed {
		memoryUsed = 0
	} else {
		memoryUsed -= valueTrackerOverhead
	}

	return memoryUsed, nSum
}
