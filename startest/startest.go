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
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

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
	maxAllocs         uint64
	maxExecutionSteps uint64
	minExecutionSteps uint64
	alive             []interface{}
	N                 int
	timerOn           bool
	timerStart        time.Time
	elapsed           time.Duration
	requiredSafety    starlark.Safety
	safetyGiven       bool
	predecls          starlark.StringDict
	locals            map[string]interface{}
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
		TestBase:          base,
		maxAllocs:         math.MaxUint64,
		maxExecutionSteps: math.MaxUint64,
	}
}

// SetMaxAllocs optionally sets the max allocations allowed per st.N.
func (st *ST) SetMaxAllocs(maxAllocs uint64) {
	st.maxAllocs = maxAllocs
}

// SetMaxExecutionSteps optionally sets the max execution steps allowed per st.N.
func (st *ST) SetMaxExecutionSteps(maxExecutionSteps uint64) {
	st.maxExecutionSteps = maxExecutionSteps
}

// SetMinExecutionSteps optionally sets the min execution steps allowed per st.N.
func (st *ST) SetMinExecutionSteps(minExecutionSteps uint64) {
	st.minExecutionSteps = minExecutionSteps
}

// RequireSafety optionally sets the required safety of tested code.
func (st *ST) RequireSafety(safety starlark.Safety) {
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

func (st *ST) StartTimer() {
	if !st.timerOn {
		st.timerOn = true
		st.timerStart = time.Now()
	}
}

func (st *ST) StopTimer() {
	if st.timerOn {
		st.elapsed += time.Since(st.timerStart)
		st.timerOn = false
	}
}

func (st *ST) ResetTimer() {
	if st.timerOn {
		st.timerStart = time.Now()
	}
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

// RunThread tests a function which has access to a Starlark thread.
func (st *ST) RunThread(fn func(*starlark.Thread)) {
	if !st.safetyGiven {
		st.requiredSafety = stSafe
	}
	thread := &starlark.Thread{}
	thread.PreallocateFrames(100)
	thread.RequireSafety(st.requiredSafety)
	thread.Print = func(_ *starlark.Thread, msg string) {
		st.Log(msg)
	}
	for k, v := range st.locals {
		thread.SetLocal(k, v)
	}

	execution := st.measureExecution(fn, thread)
	if st.Failed() {
		return
	}

	meanMeasuredAllocs := execution.allocSum / execution.nSum
	meanDeclaredAllocs := thread.Allocs() / execution.nSum
	meanExecutionSteps := thread.ExecutionSteps() / execution.nSum

	if st.maxAllocs != math.MaxUint64 && meanMeasuredAllocs > st.maxAllocs {
		st.Errorf("measured memory is above maximum (%d > %d)", meanMeasuredAllocs, st.maxAllocs)
	}

	if st.requiredSafety.Contains(starlark.MemSafe) {
		if meanDeclaredAllocs > st.maxAllocs {
			st.Errorf("declared allocations are above maximum (%d > %d)", meanDeclaredAllocs, st.maxAllocs)
		}

		if meanMeasuredAllocs > meanDeclaredAllocs {
			st.Errorf("measured memory is above declared allocations (%d > %d)", meanMeasuredAllocs, meanDeclaredAllocs)
		}
	}

	if st.maxExecutionSteps != math.MaxUint64 && meanExecutionSteps > st.maxExecutionSteps {
		st.Errorf("execution steps are above maximum (%d > %d)", meanExecutionSteps, st.maxExecutionSteps)
	}

	if meanExecutionSteps < st.minExecutionSteps {
		st.Errorf("execution steps are below minimum (%d < %d)", meanExecutionSteps, st.minExecutionSteps)
	}

	if st.requiredSafety.Contains(starlark.CPUSafe) {
		if execution.cpuDangerous && st.maxExecutionSteps == math.MaxUint64 {
			st.Errorf("execution seems to use CPU resources that are not accounted for")
		}
	}
}

// KeepAlive causes the memory of the passed objects to be measured.
func (st *ST) KeepAlive(values ...interface{}) {
	st.alive = append(st.alive, values...)
}

type executionStats struct {
	nSum, allocSum uint64
	cpuDangerous   bool
}

const (
	b_0 = 5.542717210280681e-03
	b_1 = 1.108543442056136e-02
	b_2 = 5.542717210280681e-03

	a_1 = -1.778631777824585
	a_2 = 0.800802646665708
)

type iir struct {
	w [2]float64
}

func (f *iir) Apply(x float64) float64 {
	y := f.w[0] + b_0*x
	return y
}

func (f *iir) Update(x, y float64) {
	f.w[0] = f.w[1] - a_1*y + b_1*x
	f.w[1] = b_2*x - a_2*y
}

func (f *iir) ApplyUpdate(x float64) float64 {
	y := f.Apply(x)
	f.Update(x, y)
	return y
}

func refilter(x []float64) []float64 {
	const (
		si_0 = 0.994457282789719
		si_1 = -0.795259929455426
	)

	if len(x) < 7 {
		return x // too small
	}

	filter := iir{w: [2]float64{si_0 * (x[0]*2 - x[6]), si_1 * (x[0]*2 - x[6])}}

	v := []float64{}

	for i := 6; i >= 1; i-- {
		v = append(v, filter.ApplyUpdate(x[0]*2-x[i]))
	}
	for _, x_i := range x {
		v = append(v, filter.ApplyUpdate(x_i))
	}
	for i := 1; i <= 6; i++ {
		v = append(v, filter.ApplyUpdate(x[len(x)-1]*2-x[len(x)-1-i]))
	}

	filter = iir{w: [2]float64{si_0 * v[len(v)-1], si_1 * v[len(v)-1]}}
	for i := len(v) - 1; i >= 0; i-- {
		v[i] = filter.ApplyUpdate(v[i])
	}

	return v[6 : len(x)+6]
}

func (st *ST) measureExecution(fn func(*starlark.Thread), thread *starlark.Thread) executionStats {
	startNano := time.Now().Nanosecond()

	const nMax = 100_000
	const memoryMax = 100 * 2 << 20
	const timeMax = 1e9

	var allocSum uint64
	var valueTrackerOverhead uint64
	nSum := uint64(0)
	cpuDangerous := false
	retried := false
	lastRetry := float64(0)
	lastElapsed := float64(0)
	timeSamples := []float64{}
	ns := []int{}

	for prevN := 0; !st.Failed() && allocSum-valueTrackerOverhead < memoryMax && prevN < nMax && (time.Now().Nanosecond()-startNano) < timeMax; {
	retry:
		prevMemory := allocSum
		if prevMemory <= 0 {
			prevMemory = 1
		}
		n := int(uint64(memoryMax) * uint64(prevN) / prevMemory)
		n += n / 5
		maxGrowth := prevN * 2
		minGrowth := prevN + 1
		if n > maxGrowth {
			n = maxGrowth
		} else if n < minGrowth {
			n = minGrowth
		}

		if n > nMax {
			n = nMax
		}

		st.elapsed = 0
		st.N = n

		prevAllocs := thread.Allocs()
		prevSteps := thread.ExecutionSteps()

		var before, after runtime.MemStats
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)

		runtime.LockOSThread()
		st.StartTimer()
		fn(thread)
		st.StopTimer()
		runtime.UnlockOSThread()

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&after)

		elapsed := float64(st.elapsed)

		if prevN > 1 {
			maxNegligibleElapsed := float64(lastElapsed) * math.Log(float64(n)) / math.Log(float64(prevN))
			if elapsed > maxNegligibleElapsed {
				if !retried {
					st.alive = nil
					if delta := int64(thread.Allocs()) - int64(prevAllocs); delta > 0 {
						thread.AddAllocs(-delta)
					}
					if delta := int64(thread.ExecutionSteps()) - int64(prevSteps); delta > 0 {
						thread.AddExecutionSteps(-delta)
					}

					retried = true
					lastRetry = elapsed
					goto retry
				} else {
					if lastRetry < elapsed {
						elapsed = lastRetry
					}
				}
			}
		}

		timeSamples = append(timeSamples, elapsed)
		ns = append(ns, n)
		retried = false

		if measuredAllocs := int64(after.Alloc - before.Alloc); measuredAllocs > 0 {
			allocSum += uint64(measuredAllocs)
		}

		nSum += uint64(n)
		prevN = n
		lastElapsed = elapsed
		valueTrackerOverhead += uint64(starlark.EstimateMakeSize([]interface{}{}, cap(st.alive)))
		st.alive = nil
	}

	filtered := refilter(timeSamples)
	for i := 1; i < len(filtered); i++ {
		maxNegligibleElapsed := float64(filtered[i-1]) * math.Log(float64(ns[i])) / math.Log(float64(ns[i-1]))
		if filtered[i] > maxNegligibleElapsed {
			cpuDangerous = true
		}
	}

	if st.Failed() {
		return executionStats{}
	}

	if valueTrackerOverhead > allocSum {
		allocSum = 0
	} else {
		allocSum -= valueTrackerOverhead
	}

	return executionStats{
		nSum:         nSum,
		allocSum:     allocSum,
		cpuDangerous: cpuDangerous,
	}
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

func st_ntimes(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("%s: unexpected positional arguments", b.Name())
	}
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("%s: unexpected keyword arguments", b.Name())
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
	return &ntimes_iterator{it.n}
}

type ntimes_iterator struct {
	n int
}

var _ starlark.SafeIterator = &ntimes_iterator{}

func (it *ntimes_iterator) Safety() starlark.Safety            { return stSafe }
func (it *ntimes_iterator) BindThread(thread *starlark.Thread) {}
func (it *ntimes_iterator) Done()                              {}
func (it *ntimes_iterator) Err() error                         { return nil }
func (it *ntimes_iterator) Next(p *starlark.Value) bool {
	if it.n > 0 {
		it.n--
		*p = starlark.None
		return true
	}
	return false
}
