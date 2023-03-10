// Package startest provides a framework to test Starlark code, environments
// and their safety.
//
// This framework is designed to hook into existing test frameworks such as
// testing and go-check so it can be used to write unit tests for Starlark
// usage.
//
// When a test is run, the startest instance exposes an integer N which must be
// used to scale the total resources used by the test. All checks are done in
// terms of this N, so for example, calling SetMaxAllocs(100) on a startest
// instance will cause it to check that no more than 100 allocations are made
// per N. Tests are repeated with different values of N to reduce the effect of
// noise on measurements.
//
// To create a new startest instance, use From. To test a string of Starlark
// code, use the instances's RunString method. To directly test Starlark (or
// something more expressible in Go), use the RunThread method. To simulate the
// running environment of a Starlark script, use the AddValue, AddBuiltin and
// AddLocal methods. All safety conditions are required by default; to instead
// test a specific subset of safety conditions, use the RequireSafety method.
// To test resource usage, use the SetMaxAllocs method. To count the memory
// cost of a value in a test, use the KeepAlive method. The Error, Errorf,
// Fatal, Fatalf, Log and Logf methods are inherited from the test's base.
//
// When RunString is used, testing functionality from the test is exposed to
// the Starlark code. To access the exposed N, use st.n. To count the memory
// cost of a particular value, use st.keep_alive. To report errors, use
// st.error or st.fatal. To write to the log, use the print builtin. To make
// assertions use the provided assert.* functions such as assert.eq and
// assert.contains.
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

	"github.com/canonical/starlark/resolve"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarktest"
	"gopkg.in/check.v1"
)

type TestBase interface {
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
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

// SetMaxAllocs optionally sets the max allocations allowed per st.N.
func (st *ST) SetMaxAllocs(maxAllocs uint64) {
	st.maxAllocs = maxAllocs
}

// RequireSafety optionally sets the required safety of tested code.
func (st *ST) RequireSafety(safety starlark.Safety) {
	st.requiredSafety |= safety
	st.safetyGiven = true
}

// AddValue makes the given value accessible under the given name in the
// starlark environment used by RunString.
func (st *ST) AddValue(name string, value starlark.Value) {
	if value == nil {
		st.Errorf("AddValue expected a value: got %T", value)
		return
	}

	st.addValueUnchecked(name, value)
}

// AddBuiltin makes the given builtin available under the name specified in its
// Name method in the starlark environment used by RunString.
func (st *ST) AddBuiltin(fn starlark.Value) {
	builtin, ok := fn.(*starlark.Builtin)
	if !ok {
		st.Errorf("AddBuiltin expected a builtin: got %v", fn)
		return
	}

	st.addValueUnchecked(builtin.Name(), builtin)
}

func (st *ST) addValueUnchecked(name string, value starlark.Value) {
	if st.predecls == nil {
		st.predecls = make(starlark.StringDict)
	}
	st.predecls[name] = value
}

// AddLocal adds the given object into the local values available to spawned
// threads.
func (st *ST) AddLocal(name string, value interface{}) {
	if st.locals == nil {
		st.locals = make(map[string]interface{})
	}
	st.locals[name] = value
}

// RunString tests a string of starlark code. On unexpected error, reports it,
// marks the test as failed and returns !ok. Otherwise returns ok.
func (st *ST) RunString(code string) (ok bool) {
	if code = strings.TrimRight(code, " \t\r\n"); code == "" {
		return true
	}
	code, err := Reindent(code)
	if err != nil {
		st.Error(err)
		return false
	}

	allowGlobalReassign := resolve.AllowGlobalReassign
	defer func() {
		resolve.AllowGlobalReassign = allowGlobalReassign
	}()
	resolve.AllowGlobalReassign = true

	assertMembers, err := starlarktest.LoadAssertModule()
	if err != nil {
		st.Errorf("internal error: %v", err)
		return false
	}
	assert, ok := assertMembers["assert"]
	if !ok {
		st.Errorf("internal error: no 'assert' defined in assert module")
		return false
	}

	st.AddValue("st", st)
	st.AddLocal("Reporter", st) // Set starlarktest reporter outside of RunThread
	st.AddValue("assert", assert)

	_, mod, err := starlark.SourceProgram("startest.RunString", code, func(name string) bool {
		_, ok := st.predecls[name]
		return ok
	})
	if err != nil {
		st.Error(err)
		return false
	}

	var codeErr error
	st.RunThread(func(thread *starlark.Thread) {
		// Continue RunThread's test loop
		if codeErr != nil {
			return
		}
		_, codeErr = mod.Init(thread, st.predecls)
	})
	if codeErr != nil {
		st.Error(codeErr)
	}
	return codeErr == nil
}

// RunThread tests a function which has access to a starlark thread.
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

// KeepAlive causes the memory of the passed objects to be measured.
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

func (st *ST) String() string        { return "<startest.ST>" }
func (st *ST) Type() string          { return "startest.ST" }
func (st *ST) Freeze()               { st.predecls.Freeze() }
func (st *ST) Truth() starlark.Bool  { return starlark.True }
func (st *ST) Hash() (uint32, error) { return 0, errors.New("unhashable type: startest.ST") }

var errorMethod = starlark.NewBuiltinWithSafety("error", stSafe, st_error)
var fatalMethod = starlark.NewBuiltinWithSafety("fatal", stSafe, st_fatal)
var keepAliveMethod = starlark.NewBuiltinWithSafety("keep_alive", stSafe, st_keep_alive)

func (st *ST) Attr(name string) (starlark.Value, error) {
	switch name {
	case "error":
		return errorMethod.BindReceiver(st), nil
	case "fatal":
		return fatalMethod.BindReceiver(st), nil
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
		"fatal",
		"keep_alive",
		"n",
	}
}

// st_error logs the passed starlark objects as errors in the current test.
func st_error(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) != 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
	}

	recv := b.Receiver().(*ST)
	recv.Error(errReprs(args)...)
	return starlark.None, nil
}

// st_fatal logs the passed starlark objects as errors in the current test
// before aborting it.
func st_fatal(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) != 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
	}

	recv := b.Receiver().(*ST)
	recv.Fatal(errReprs(args)...)
	panic(fmt.Sprintf("internal error: %T.Fatal returned", recv))
}

func errReprs(args []starlark.Value) []interface{} {
	reprs := make([]interface{}, 0, len(args))
	for _, arg := range args {
		var repr string
		if s, ok := arg.(starlark.String); ok {
			repr = s.GoString()
		} else {
			repr = arg.String()
		}
		reprs = append(reprs, repr)
	}
	return reprs
}

// st_keep_alive prevents the memory of the passed starlark objects being
// freed. This forces the current test to measure these objects' memory.
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
