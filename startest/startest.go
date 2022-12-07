package startest

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarktest"
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
	predecls       starlark.StringDict
	locals         map[string]interface{}
	TestBase
}

const stSafe = starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe | starlark.IOSafe

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

// AddValue adds the given starlark Value into the starlark environment used by
// RunString.
func (st *ST) AddValue(name string, value starlark.Value) {
	if st.predecls == nil {
		st.predecls = make(starlark.StringDict)
	}
	st.predecls[name] = value
}

// AddBuiltin adds the given builtin into the starlark environment used by
// RunString under the name specified in its Name method.
func (st *ST) AddBuiltin(fn starlark.Value) {
	builtin, ok := fn.(*starlark.Builtin)
	if !ok {
		st.Errorf("AddBuiltin expected a builtin: got %v", fn)
		return
	}

	st.AddValue(builtin.Name(), builtin)
}

// AddLocal adds the given object into the local values available to spawned
// threads.
func (st *ST) AddLocal(name string, value interface{}) {
	if st.locals == nil {
		st.locals = make(map[string]interface{})
	}
	st.locals[name] = value
}

// RunString tests a string of starlark code.
func (st *ST) RunString(code string) error {
	code = strings.TrimRight(code, " \t\r\n")

	if code == "" {
		return nil
	}

	sb := strings.Builder{}
	sb.Grow(len(code))
	sb.WriteString("load('assert.star', 'assert')\n")
	sb.WriteString("def __test__():\n")

	isNewlineRune := func(r rune) bool { return r == '\r' || r == '\n' }
	lines := strings.FieldsFunc(code, isNewlineRune)

	if len(lines) > 1 && !isNewlineRune(rune(code[0])) {
		st.Errorf(`Multi-line snippets should start with a newline: got "%s"`, lines[0])
		return errors.New("internal error")
	}

	for _, line := range lines {
		sb.WriteRune('\t')
		sb.WriteString(line)
		sb.WriteRune('\n')
	}
	sb.WriteString("__test__()")

	code = sb.String()

	assert, err := starlarktest.LoadAssertModule()
	if err != nil {
		st.Error(err)
		return errors.New("internal error")
	}

	st.AddValue("st", st)
	st.AddLocal("Reporter", st) // Set starlarktest reporter outside of RunThread

	_, mod, err := starlark.SourceProgram("startest.RunString", code, func(name string) bool {
		_, ok := st.predecls[name]
		if !ok {
			_, ok = assert[name]
		}
		return ok
	})
	if err != nil {
		st.Error(err)
		return errors.New("internal error")
	}

	var codeErr error
	st.RunThread(func(thread *starlark.Thread) {
		// Continue RunThread's test loop
		if codeErr != nil {
			return
		}
		thread.Load = func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
			if module == "assert.star" {
				return assert, nil
			}
			panic("unreachable")
		}
		_, codeErr = mod.Init(thread, st.predecls)
	})
	return codeErr
}

// RunThread tests a function which has access to a starlark thread and a global environment
func (st *ST) RunThread(fn func(*starlark.Thread)) {
	if !st.safetyGiven {
		st.requiredSafety = stSafe
	}

	thread := &starlark.Thread{}
	thread.RequireSafety(st.requiredSafety)
	thread.Print = func(_ *starlark.Thread, msg string) {
		st.Log(msg)
	}
	for k, v := range st.locals {
		thread.SetLocal(k, v)
	}

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
func (st *ST) Freeze()               { st.predecls.Freeze() }
func (st *ST) Truth() starlark.Bool  { return starlark.True }
func (st *ST) Hash() (uint32, error) { return 0, errors.New("unhashable type: startest.ST") }

var keepAliveMethod = starlark.NewBuiltinWithSafety("keep_alive", stSafe, st_keep_alive)

func (st *ST) Attr(name string) (starlark.Value, error) {
	switch name {
	case "keep_alive":
		return keepAliveMethod.BindReceiver(st), nil
	case "n":
		return starlark.MakeInt(st.N), nil
	}
	return nil, nil
}

func (*ST) AttrNames() []string {
	return []string{
		"keep_alive",
		"n",
	}
}

// st_keep_alive causes the memory of the passed starlark objects to be
// measured.
func st_keep_alive(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
	}

	recv := b.Receiver().(*ST)
	for _, arg := range args {
		recv.KeepAlive(arg)
	}

	return starlark.None, nil
}
