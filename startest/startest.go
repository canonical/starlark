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

// SetMaxAllocs optionally sets the max allocations allowed per st.N
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
// RunString under the name specified in its Name() method.
func (st *ST) AddBuiltin(fn starlark.Value) {
	builtin, ok := fn.(*starlark.Builtin)
	if !ok {
		st.Error("AddBuiltin expected a builtin: got %v", fn)
		return
	}

	st.AddValue(builtin.Name(), builtin)
}

// RunString tests a string of starlark code
func (st *ST) RunString(code string) {
	sb := strings.Builder{}
	sb.Grow(len(code))
	sb.WriteString("def __test__():\n")
	code = strings.TrimRight(code, " \t\n")

	// Unindent code
	var baseIndent string
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if i == 0 {
			line = strings.TrimRight(line, " \t")
			if line == "" {
				continue
			} else if len(lines) > 1 {
				st.Error("Multi-line snippets should start with a newline")
				return
			}
		}

		if i == 1 {
			for i, c := range line {
				if c != ' ' && c != '\t' {
					baseIndent = line[:i]
					break
				}
			}
		} else if (i > 1 && i < len(lines)-1) && len(line) != 0 && !strings.HasPrefix(line, baseIndent) {
			st.Errorf("Expected prefix %#v in line %#v", baseIndent, line)
			return
		}

		if len(baseIndent) <= len(line) {
			sb.WriteString("\t")
			sb.WriteString(line[len(baseIndent):])
			sb.WriteRune('\n')
		}
	}
	if code == "" {
		sb.WriteString("\tpass\n")
	}
	sb.WriteString("__test__()")

	code = sb.String()

	st.AddValue("st", st)
	_, mod, err := starlark.SourceProgram("startest.RunString", code, func(name string) bool {
		_, ok := st.predecls[name]
		return ok
	})
	if err != nil {
		st.Error(err)
		return
	}

	st.RunThread(func(thread *starlark.Thread) {
		if _, err := mod.Init(thread, st.predecls); err != nil {
			st.Error(err)
		}
	})
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

var errorMethod = starlark.NewBuiltinWithSafety("error", stSafe, st_error)
var keepAliveMethod = starlark.NewBuiltinWithSafety("keep_alive", stSafe, st_keep_alive)

func (st *ST) Attr(name string) (starlark.Value, error) {
	switch name {
	case "error":
		return errorMethod.BindReceiver(st), nil
	case "keep_alive":
		return keepAliveMethod.BindReceiver(st), nil
	case "n":
		return starlark.MakeInt(st.N), nil
	}
	return nil, nil
}

func (*ST) AttrNames() []string {
	return []string{
		"error",
		"keep_alive",
		"n",
	}
}

func st_error(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	buf := &strings.Builder{}
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
	}

	for i, arg := range args {
		if i > 0 {
			buf.WriteRune(' ')
		}

		if s, ok := starlark.AsString(arg); ok {
			buf.WriteString(s)
		} else if b, ok := arg.(starlark.Bytes); ok {
			buf.WriteString(string(b))
		} else {
			buf.WriteString(arg.String())
		}
	}

	st := b.Receiver().(*ST)
	st.Error(buf.String())

	return starlark.None, nil
}

// st_keep_alive causes the memory of the passed starlark objects to be
// measured
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
