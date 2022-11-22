package startest

import (
	"errors"
	"math"
	"runtime"
	"strings"
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

type ST struct {
	maxAllocs      uint64
	alive          []interface{}
	N              int
	requiredSafety starlark.Safety
	safetyGiven    bool
	TestBase
}

var _ starlark.Value = &ST{}
var _ starlark.HasAttrs = &ST{}

var _ TestBase = &testing.T{}
var _ TestBase = &testing.B{}
var _ TestBase = &check.C{}

// From returns a new starTest instance with a given test base.
func From(base TestBase) *ST {
	return &ST{TestBase: base, maxAllocs: math.MaxUint64}
}

// SetMaxAllocs optionally sets the max allocations allowed per test.N
func (st *ST) SetMaxAllocs(maxAllocs uint64) {
	st.maxAllocs = maxAllocs
}

// RequireSafety optionally sets the required safety of tested code
func (st *ST) RequireSafety(safety starlark.Safety) {
	st.requiredSafety |= safety
	st.safetyGiven = true
}

func (st *ST) RunString(code string) {
	sb := strings.Builder{}
	sb.Grow(len(code))

	// Clean code
	var baseIndent string
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if i == 0 && line == "" {
			continue
		}

		if i == 1 {
			for i, c := range line {
				if c != ' ' && c != '\t' {
					baseIndent = line[:i]
					break
				}
			}
		} else if (i > 1 && i < len(lines)-1) && !strings.HasPrefix(line, baseIndent) {
			st.Errorf("Expected prefix %#v in line %#v", baseIndent, line)
			return
		}

		if len(baseIndent) <= len(line) {
			sb.WriteString(line[len(baseIndent):])
			sb.WriteRune('\n')
		}
	}
	code = sb.String()

	st.Errorf("%#v", code)
	st.RunThread(func(thread *starlark.Thread) {
		predecls := starlark.StringDict{
			"st": st,
		}
		_, err := starlark.ExecFile(thread, "startest.RunString", code, predecls)
		if err != nil {
			st.Error(err)
		}
	})
}

// RunThread tests a function which has access to a starlark thread and a global environment
func (st *ST) RunThread(fn func(*starlark.Thread)) {
	if !st.safetyGiven {
		st.requiredSafety = starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe | starlark.IOSafe
	}

	thread := &starlark.Thread{}
	thread.RequireSafety(st.requiredSafety)

	memorySum, nSum := st.measureMemory(func() {
		fn(thread)
	})

	if st.Failed() {
		return
	}

	meanMeasured := memorySum / nSum
	meanDeclared := thread.Allocs() / nSum

	if st.maxAllocs != math.MaxUint64 && meanMeasured > st.maxAllocs {
		st.Errorf("measured memory is above maximum (%d > %d)", meanMeasured, st.maxAllocs)
	}

	if st.requiredSafety.Contains(starlark.MemSafe) {
		if meanDeclared > st.maxAllocs {
			st.Errorf("declared allocations are above maximum (%d > %d)", meanDeclared, st.maxAllocs)
		}

		if meanMeasured > meanDeclared {
			st.Errorf("measured memory is above declared allocations (%d > %d)", meanMeasured, meanDeclared)
		}
	}
}

// KeepAlive causes the memory of the passed objects to be measured
func (st *ST) KeepAlive(values ...interface{}) {
	st.alive = append(st.alive, values...)
}

func (st *ST) measureMemory(fn func()) (memorySum, nSum uint64) {
	startNano := time.Now().Nanosecond()

	const nMax = 100_000
	const memoryMax = 100 * 2 << 20
	const timeMax = 1e9

	var memoryUsed uint64
	var valueTrackerOverhead uint64
	st.N = 0
	nSum = 0

	for n := uint64(0); !st.Failed() && memoryUsed-valueTrackerOverhead < memoryMax && n < nMax && (time.Now().Nanosecond()-startNano) < timeMax; {
		last := n
		prevIters := uint64(st.N)
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

		st.N = int(n)
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
		valueTrackerOverhead += uint64(cap(st.alive)) * uint64(unsafe.Sizeof(interface{}(nil)))
		st.alive = nil
		if iterationMeasure > 0 {
			memoryUsed += uint64(iterationMeasure)
		}
	}

	if st.Failed() {
		return 0, 1
	}

	if valueTrackerOverhead > memoryUsed {
		memoryUsed = 0
	} else {
		memoryUsed -= valueTrackerOverhead
	}

	return memoryUsed, nSum
}

func (st *ST) String() string        { return "startest instance" }
func (st *ST) Type() string          { return "startest.ST" }
func (st *ST) Freeze()               {}
func (st *ST) Truth() starlark.Bool  { return starlark.True }
func (st *ST) Hash() (uint32, error) { return 0, errors.New("unhashable type: startest.ST") }
