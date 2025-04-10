// Package startest provides a framework to test Starlark code, environments
// and their safety.
//
// This framework is designed to hook into existing test frameworks, such as
// testing and go-check, so it can be used to write unit tests for Starlark
// usage.
//
// When a test is run, the startest instance exposes an integer N which must be
// used to scale the total resources used by the test. All checks are done in
// terms of this N, so for example, calling SetMaxAllocs(100) on a startest
// instance will cause it to check that no more than 100 bytes are allocated
// per given N. Tests are repeated with different values of N to reduce the
// effect of noise on measurements.
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
// When executing Starlark code, the startest instance can be accessed through
// the global st. To access the exposed N, use st.n. To count the memory cost
// of a particular value, use st.keep_alive. To report errors, use st.error or
// st.fatal. To write to the log, use the print builtin. To ergonomically make
// assertions, use the provided assert global which provides functions such as
// assert.eq, assert.true and assert.fails.
package startest

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"runtime/metrics"
	"strings"
	"testing"
	"time"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarktest"
	"github.com/canonical/starlark/syntax"
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
	ctx            context.Context
	maxAllocs      int64
	maxSteps       int64
	minSteps       int64
	alive          []interface{}
	N              int
	requiredSafety starlark.SafetyFlags
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
	return &ST{
		TestBase:  base,
		ctx:       context.Background(),
		maxAllocs: math.MaxInt64,
		maxSteps:  math.MaxInt64,
	}
}

// SetParentContext optionally sets the parent context of startest threads.
func (st *ST) SetParentContext(ctx context.Context) {
	st.ctx = ctx
}

// SetMaxAllocs optionally sets the max allocations allowed per unit of st.N.
func (st *ST) SetMaxAllocs(maxAllocs int64) {
	st.maxAllocs = maxAllocs
}

// SetMaxSteps optionally sets the max steps allowed per unit
// of st.N.
func (st *ST) SetMaxSteps(maxSteps int64) {
	st.maxSteps = maxSteps
}

// SetMinSteps optionally sets the min steps allowed per unit
// of st.N.
func (st *ST) SetMinSteps(minSteps int64) {
	st.minSteps = minSteps
}

// RequireSafety optionally sets the required safety of tested code.
func (st *ST) RequireSafety(safety starlark.SafetyFlags) {
	st.requiredSafety |= safety
	st.safetyGiven = true
}

// AddValue makes the given value accessible under the given name in the
// Starlark environment used by RunString.
func (st *ST) AddValue(name string, value starlark.Value) {
	if value == nil {
		st.Errorf("AddValue expected a value: got %T", value)
		return
	}

	st.addValueUnchecked(name, value)
}

// AddBuiltin makes the given builtin available under the name specified in its
// Name method in the Starlark environment used by RunString.
func (st *ST) AddBuiltin(fn starlark.Value) {
	builtin, ok := fn.(*starlark.Builtin)
	if !ok {
		st.Errorf("AddBuiltin expected a builtin: got %T", fn)
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

// RunString tests a string of Starlark code. On unexpected error, reports it,
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

	options := &syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}

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

	_, mod, err := starlark.SourceProgramOptions(options, "startest.RunString", code, func(name string) bool {
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

// RunThread tests a function which has access to a Starlark thread.
func (st *ST) RunThread(fn func(*starlark.Thread)) {
	if !st.safetyGiven {
		st.requiredSafety = stSafe
	}

	thread := &starlark.Thread{}
	thread.SetParentContext(st.ctx)
	thread.EnsureStack(100)
	thread.RequireSafety(st.requiredSafety)
	thread.Print = func(_ *starlark.Thread, msg string) {
		st.Log(msg)
	}
	for k, v := range st.locals {
		thread.SetLocal(k, v)
	}

	stats := st.measureExecution(thread, fn)
	if st.Failed() {
		return
	}

	mean := func(x int64) int64 { return (x + stats.nSum/2) / stats.nSum }
	meanMeasuredAllocs := mean(stats.allocSum)
	allocs64, ok := thread.Allocs()
	if !ok {
		st.Error("alloc counter invalidated")
		return
	}
	meanDeclaredAllocs := mean(allocs64)
	steps64, ok := thread.Steps()
	if !ok {
		st.Error("step counter invalidated")
		return
	}
	meanSteps := mean(steps64)

	if st.maxAllocs != math.MaxInt64 && st.maxAllocs >= 0 && meanMeasuredAllocs > st.maxAllocs {
		st.Errorf("measured memory is above maximum (%d > %d)", meanMeasuredAllocs, st.maxAllocs)
	}
	if st.requiredSafety.Contains(starlark.MemSafe) {
		if meanDeclaredAllocs > st.maxAllocs {
			st.Errorf("declared allocations are above maximum (%d > %d)", meanDeclaredAllocs, st.maxAllocs)
		}

		// Check memory usage is safe, within mean rounding error (i.e. round(alloc error per N) == 0)
		allocs, ok := thread.Allocs()
		if !ok {
			st.Error("alloc counter invalidated")
		}
		if stats.allocSum > allocs && (stats.allocSum-allocs)*2 >= stats.nSum {
			st.Errorf("measured memory is above declared allocations (%d > %d)", meanMeasuredAllocs, meanDeclaredAllocs)
		}
	}

	if st.maxSteps != math.MaxInt64 && st.maxSteps >= 0 && meanSteps > st.maxSteps {
		st.Errorf("steps are above maximum (%d > %d)", meanSteps, st.maxSteps)
	}
	if meanSteps < st.minSteps {
		st.Errorf("steps are below minimum (%d < %d)", meanSteps, st.minSteps)
	}
	if st.requiredSafety.Contains(starlark.CPUSafe) {
		if stats.stepsRequired && meanSteps == 0 {
			st.Errorf("execution uses CPU time which is not accounted for")
		}
	}
}

// KeepAlive causes the memory of the passed objects to be measured.
func (st *ST) KeepAlive(values ...interface{}) {
	st.alive = append(st.alive, values...)
}

type runStats struct {
	nSum, allocSum int64
	stepsRequired  bool
}

func (st *ST) measureExecution(thread *starlark.Thread, fn func(*starlark.Thread)) runStats {
	const nMax = 100_000
	const memoryMax = 200 * (1 << 20)
	const timeMax = time.Second

	nSum := int64(0)
	allocSum, valueTrackerAllocs := starlark.SafeInt(0), starlark.SafeInt(0)

	startTime := time.Now()
	prevN, elapsed := int64(0), time.Duration(0)
	for {
		if allocSum64, ok := allocSum.Int64(); !ok {
			st.Error("memory limit invalidated")
			return runStats{}
		} else if memoryLimit64, ok := starlark.SafeAdd(memoryMax, valueTrackerAllocs).Int64(); !ok {
			st.Error("memory limit invalidated")
			return runStats{}
		} else if allocSum64 >= memoryLimit64 || prevN >= nMax || elapsed >= timeMax {
			break
		}

		var n int64
		if nSum != 0 {
			n = prevN * 2

			allocsPerN := starlark.SafeDiv(allocSum, nSum)
			if allocsPerN == starlark.SafeInt(0) {
				allocsPerN = starlark.SafeInt(1)
			}
			memoryLimitN, ok := starlark.SafeDiv(starlark.SafeSub(memoryMax, allocSum), allocsPerN).Int64()
			if !ok {
				st.Error("memory limit invalidated")
				return runStats{}
			}
			if n > memoryLimitN {
				n = memoryLimitN
			}

			timePerN := elapsed / time.Duration(nSum)
			if timePerN == 0 {
				timePerN = 1
			}
			timeLimitN := int64((timeMax - elapsed) / timePerN)
			if n > timeLimitN {
				n = timeLimitN
			}
		}
		if n <= 0 {
			n = 1
		} else if n > nMax {
			n = nMax
		}

		var alive []interface{}
		if st.requiredSafety.Contains(starlark.MemSafe) {
			alive = make([]interface{}, 0, n)
		} else {
			alive = make([]interface{}, 0, 1)
		}

		st.alive = alive
		st.N = int(n)

		beforeAllocs := readMemoryUsage(st.requiredSafety.Contains(starlark.MemSafe))
		fn(thread)
		afterAllocs := readMemoryUsage(st.requiredSafety.Contains(starlark.MemSafe))

		runtime.KeepAlive(alive)

		if st.Failed() {
			return runStats{}
		}

		// If st.alive was reallocated, the cost of its new memory block is
		// included in the measurement. This overhead must be discounted
		// when reasoning about the measurement.
		if cap(st.alive) != cap(alive) {
			valueTrackerAllocs = starlark.SafeAdd(valueTrackerAllocs, starlark.EstimateMakeSize([]interface{}{}, starlark.SafeInt(cap(st.alive))))
		}
		if afterAllocs > beforeAllocs {
			allocSum = starlark.SafeAdd(allocSum, starlark.SafeSub(afterAllocs, beforeAllocs))
		}

		nSum += n
		prevN = n
		elapsed = time.Since(startTime)
		st.alive = nil
	}

	if allocSum64, ok := allocSum.Int64(); !ok {
		st.Error("alloc count invalidated")
		return runStats{}
	} else if valueTrackerAllocs64, ok := valueTrackerAllocs.Int64(); !ok {
		st.Error("value tracker alloc count invalidated")
		return runStats{}
	} else if allocSum64 < valueTrackerAllocs64 {
		allocSum = starlark.SafeInt(0)
	} else {
		allocSum = starlark.SafeSub(allocSum, valueTrackerAllocs)
	}

	timePerN := elapsed / time.Duration(nSum)
	stepsRequired := timePerN > time.Millisecond
	allocSum64, ok := allocSum.Int64()
	if !ok {
		st.Error("alloc count invalidated")
	}
	return runStats{
		nSum:          nSum,
		allocSum:      allocSum64,
		stepsRequired: stepsRequired,
	}
}

// readMemoryUsage returns the number of bytes in use by the Go runtime. If
// precise measurement is required, a full GC will be performed.
func readMemoryUsage(precise bool) int64 {
	if precise {
		// Run the GC twice to account for finalizers.
		runtime.GC()
		runtime.GC()

		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return int64(stats.Alloc)
	}

	sample := []metrics.Sample{
		{Name: "/gc/heap/allocs:bytes"},
	}
	metrics.Read(sample)
	if sample[0].Value.Kind() == metrics.KindBad {
		return 0
	}
	return int64(sample[0].Value.Uint64())
}

func (st *ST) String() string        { return "<startest.ST>" }
func (st *ST) Type() string          { return "startest.ST" }
func (st *ST) Freeze()               { st.predecls.Freeze() }
func (st *ST) Truth() starlark.Bool  { return starlark.True }
func (st *ST) Hash() (uint32, error) { return 0, errors.New("unhashable type: startest.ST") }

var errorMethod = starlark.NewBuiltinWithSafety("error", stSafe, st_error)
var fatalMethod = starlark.NewBuiltinWithSafety("fatal", stSafe, st_fatal)
var keepAliveMethod = starlark.NewBuiltinWithSafety("keep_alive", stSafe, st_keep_alive)
var ntimesMethod = starlark.NewBuiltinWithSafety("ntimes", stSafe, st_ntimes)

func (st *ST) Attr(name string) (starlark.Value, error) {
	switch name {
	case "error":
		return errorMethod.BindReceiver(st), nil
	case "fatal":
		return fatalMethod.BindReceiver(st), nil
	case "keep_alive":
		return keepAliveMethod.BindReceiver(st), nil
	case "ntimes":
		return ntimesMethod.BindReceiver(st), nil
	case "n":
		return starlark.MakeInt(st.N), nil
	}
	return nil, nil
}

func (st *ST) SafeAttr(thread *starlark.Thread, name string) (starlark.Value, error) {
	if thread != nil {
		// The steps counted to call this function is:
		// - 1 for loading st;
		// - 1 for getting the attr.
		if err := thread.AddSteps(starlark.SafeInt(-2)); err != nil {
			return nil, err
		}
	}
	return st.Attr(name) // Assume test code is safe.
}

func (st *ST) AttrNames() []string {
	return []string{
		"error",
		"fatal",
		"keep_alive",
		"n",
	}
}

// st_error logs the passed Starlark objects as errors in the current test.
func st_error(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) != 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
	}

	recv := b.Receiver().(*ST)
	recv.Error(errReprs(args)...)
	return starlark.None, nil
}

// st_fatal logs the passed Starlark objects as errors in the current test
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

// st_keep_alive prevents the memory of the passed Starlark objects being
// freed. This forces the current test to measure these objects' memory.
func st_keep_alive(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
	}

	// Remove the cost of the CALL for this function.
	if err := thread.AddSteps(starlark.SafeInt(-1)); err != nil {
		return nil, err
	}

	// keep_alive does not capture the backing array for args. Hence
	// the allocation is removed aligning declared allocations with
	// user expectations.
	argsSize := starlark.EstimateMakeSize(starlark.Tuple{}, starlark.SafeInt(cap(args)))
	if err := thread.AddAllocs(starlark.SafeNeg(argsSize)); err != nil {
		return nil, err
	}
	recv := b.Receiver().(*ST)
	for _, arg := range args {
		recv.KeepAlive(arg)
	}

	return starlark.None, nil
}

func st_ntimes(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("%s: unexpected positional arguments", b.Name())
	}
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
	}

	// Remove the cost of the CALL for this function.
	if err := thread.AddSteps(starlark.SafeInt(-1)); err != nil {
		return nil, err
	}

	recv := b.Receiver().(*ST)
	return &ntimes_iterable{recv.N}, nil
}

type ntimes_iterable struct {
	n int
}

var _ starlark.Iterable = &ntimes_iterable{}

func (it *ntimes_iterable) Freeze() {}
func (it *ntimes_iterable) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable: %s", it.Type())
}
func (it *ntimes_iterable) String() string       { return "st.ntimes()" }
func (it *ntimes_iterable) Truth() starlark.Bool { return true }
func (it *ntimes_iterable) Type() string         { return "st.ntimes" }
func (it *ntimes_iterable) Iterate() starlark.Iterator {
	return &ntimes_iterator{n: it.n}
}

type ntimes_iterator struct {
	n             int
	thread        *starlark.Thread
	err           error
	stepsToRemove starlark.SafeInteger
}

var _ starlark.SafeIterator = &ntimes_iterator{}

func (it *ntimes_iterator) Safety() starlark.SafetyFlags       { return stSafe }
func (it *ntimes_iterator) BindThread(thread *starlark.Thread) { it.thread = thread }
func (it *ntimes_iterator) Err() error                         { return it.err }

func (it *ntimes_iterator) Next(p *starlark.Value) bool {
	if it.n > 0 {
		// Counted loop iteration steps comprise:
		// - 1 for the ITERJMP;
		// - 1 for guardedIterator's Next;
		// - 1 for the SET{LOCAL,GLOBAL} to record the result;
		// - 1 for the JMP back to the loop's ITERJMP.
		it.stepsToRemove = starlark.SafeAdd(it.stepsToRemove, 4)
		it.n--
		*p = starlark.None
		return true
	}
	return false
}

func (it *ntimes_iterator) Done() {
	if it.thread == nil {
		return
	}

	err := it.thread.AddSteps(starlark.SafeNeg(it.stepsToRemove))
	if err != nil && it.err == nil {
		it.err = err
	}
}
