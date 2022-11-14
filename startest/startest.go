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

type testBase interface {
	Error(err ...interface{})
	Errorf(format string, err ...interface{})
	Failed() bool
	Log(args ...interface{})
	Logf(fmt string, args ...interface{})
}

type S struct {
	predefined starlark.StringDict
	maxAllocs  uint64
	margin     float64
	tracked    []interface{}
	N          int
	testBase
}

var _ testBase = &testing.T{}
var _ testBase = &testing.B{}
var _ testBase = &check.C{}

// From returns a new starTest instance with a given test base.
func From(base testBase) *S {
	return &S{testBase: base, maxAllocs: math.MaxUint64, margin: 0.05}
}

// AddBuiltin inserts the given Builtin into the predeclared values passed to
// RunThread.
func (test *S) AddBuiltin(fn *starlark.Builtin) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[fn.Name()] = fn
}

// AddValue inserts the given value into the predeclared values passed to
// RunThread.
func (test *S) AddValue(name string, value starlark.Value) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[name] = value
}

// SetMaxAllocs optionally sets the max allocations allowed per test.N
func (test *S) SetMaxAllocs(maxAllocs uint64) {
	test.maxAllocs = maxAllocs
}

// SetRealityMargin sets the fraction by which measured allocations can be greater
// than from declared allocations
func (test *S) SetRealityMargin(margin float64) {
	if test.margin > 0 {
		test.margin = margin
	} else {
		test.margin = 0
	}
}

// RunThread tests a function which has access to a starlark thread and a global environment
func (test *S) RunThread(fn func(*starlark.Thread, starlark.StringDict)) {
	thread := &starlark.Thread{}

	totMemory, nTotal := test.measureMemory(func() {
		fn(thread, test.predefined)
	})

	if test.Failed() {
		return
	}

	meanMeasured := totMemory / nTotal

	if meanMeasured > test.maxAllocs {
		test.Errorf("measured memory is above maximum (%d > %d)", meanMeasured, test.maxAllocs)
	}

	if meanDeclared := thread.Allocs() / nTotal; meanDeclared > test.maxAllocs {
		test.Errorf("declared allocations are above maximum (%d > %d)", meanDeclared, test.maxAllocs)
	}

	if test.maxAllocs != math.MaxUint64 {
		measuredUpperBound := (thread.Allocs() * uint64((1+test.margin)*100) / 100) / nTotal
		if meanMeasured > measuredUpperBound {
			test.Errorf("measured memory is more than %.0f%% above declared allocations (%d > %d)", test.margin*100, meanMeasured, measuredUpperBound)
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
		test.tracked = make([]interface{}, 0)
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
