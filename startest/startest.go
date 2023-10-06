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
	"math/bits"
	"runtime"
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
	maxAllocs         uint64
	maxExecutionSteps uint64
	minExecutionSteps uint64
	alive             []interface{}
	N                 int
	timerOn           bool
	timerStart        instant
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

//go:inline
func (st *ST) StartTimer() {
	if !st.timerOn {
		st.timerOn = true
		st.timerStart = nanotime()
	}
}

//go:inline
func (st *ST) StopTimer() {
	if st.timerOn {
		st.elapsed += time.Duration(nanotime() - st.timerStart)
		st.timerOn = false
	}
}

//go:inline
func (st *ST) ResetTimer() {
	if st.timerOn {
		st.timerStart = nanotime()
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

	mean := func(x uint64) uint64 { return (x + stats.nSum/2) / stats.nSum }
	meanMeasuredAllocs := mean(stats.allocSum)
	meanDeclaredAllocs := mean(thread.Allocs())
	meanExecutionSteps := mean(thread.ExecutionSteps())

	if st.maxAllocs != math.MaxUint64 && meanMeasuredAllocs > st.maxAllocs {
		st.Errorf("measured memory is above maximum (%d > %d)", meanMeasuredAllocs, st.maxAllocs)
	}
	if st.requiredSafety.Contains(starlark.MemSafe) {
		if meanDeclaredAllocs > st.maxAllocs {
			st.Errorf("declared allocations are above maximum (%d > %d)", meanDeclaredAllocs, st.maxAllocs)
		}

		// Check memory usage is safe, within mean rounding error (i.e. round(alloc error per N) == 0)
		if stats.allocSum > thread.Allocs() && (stats.allocSum-thread.Allocs())*2 >= stats.nSum {
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
		if stats.unaccountedCPUTime && st.maxExecutionSteps == math.MaxUint64 {
			st.Errorf("execution uses CPU time which is not accounted for")
		}
	}
}

// KeepAlive causes the memory of the passed objects to be measured.
func (st *ST) KeepAlive(values ...interface{}) {
	st.alive = append(st.alive, values...)
}

type executionStats struct {
	nSum, allocSum     uint64
	unaccountedCPUTime bool
}

func (st *ST) measureExecution(thread *starlark.Thread, fn func(*starlark.Thread)) executionStats {
	const nMax = 100_000
	const memoryMax = 200 * (1 << 20)
	const timeMax = time.Second
	const targetSamples = 50

	nSum, allocSum, valueTrackerAllocs := uint64(0), uint64(0), uint64(0)
	samplingInterval := int64(100)
	timeSamples := make([]float64, 0, targetSamples)

	startTime := time.Now()
	retried := false
	lastRetryElapsed := time.Duration(0)
	prevElapsed := time.Duration(0)
	prevN, elapsed := int64(0), time.Duration(0)
	for ; allocSum < memoryMax+valueTrackerAllocs && prevN < nMax && elapsed < timeMax; elapsed = time.Since(startTime) {
	retry:
		var n int64
		prevTime := st.elapsed
		if prevTime <= 0 {
			prevTime = 1
		}

		if !st.requiredSafety.Contains(starlark.CPUSafe) && !st.requiredSafety.Contains(starlark.CPUSafe) {
			// Even if there is no resource target set, it is still best to run
			// the logic few times with increasing N. It is not necessary to do
			// any time/space estimation, so N can just grow exponentially,
			// bound by nMax.
			n = prevN * 2
		} else if nSum > 0 {
			if st.requiredSafety.Contains(starlark.MemSafe) {
				allocsPerN := int64(uint64(allocSum) / nSum)
				if allocsPerN <= 0 {
					allocsPerN = 1
				}
				memoryTargetN := (memoryMax - int64(allocSum)) / allocsPerN
				if n < memoryTargetN {
					n = prevN * 2 // max growth
					if memoryTargetN < n {
						n = memoryTargetN
					}
				}
			}

			if st.requiredSafety.Contains(starlark.CPUSafe) {
				if len(timeSamples) >= targetSamples {
					// Check if there's enough time to resample with a bigger sampling interval
					timePerN := elapsed / time.Duration(nSum)
					remainingTime := timeMax - elapsed
					resampleCount := int64(targetSamples / 2)
					totalN := resampleCount*int64(prevN) + samplingInterval*resampleCount*(resampleCount-1)/2
					if remainingTime < time.Duration(totalN)*timePerN {
						break
					}
					samplingInterval *= 2
					// Doubling the sampling interval means that some of old
					// samples will be discarded (the odd ones!).
					timeSamples = decimate(timeSamples)
				}
				n = prevN + samplingInterval
			}
		}

		if n <= prevN {
			n = prevN + 1
		}

		alive := make([]interface{}, 0, n)
		st.alive = alive
		st.elapsed = 0
		st.N = int(n)

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
		runtime.KeepAlive(alive)

		if st.Failed() {
			return executionStats{}
		}

		if st.requiredSafety.Contains(starlark.CPUSafe) && prevElapsed > 0 {
			if st.elapsed > prevElapsed*2 {
				// Confirm that the quick increase in time comes from the
				// function execution and not external factors.
				if !retried {
					st.alive = nil
					if delta := int64(thread.Allocs()) - int64(prevAllocs); delta > 0 {
						thread.AddAllocs(-delta)
					}
					if delta := int64(thread.ExecutionSteps()) - int64(prevSteps); delta > 0 {
						thread.AddExecutionSteps(-delta)
					}

					lastRetryElapsed = st.elapsed
					retried = true
					goto retry
				}

				if lastRetryElapsed < st.elapsed {
					st.elapsed = lastRetryElapsed
				}
			}
		}

		// If st.alive was reallocated, the cost of its new memory block is
		// included in the measurement. This overhead must be discounted
		// when reasoning about the measurement.
		if cap(st.alive) != cap(alive) {
			valueTrackerAllocs += uint64(starlark.EstimateMakeSize([]interface{}{}, cap(st.alive)))
		}
		if after.Alloc > before.Alloc {
			allocSum += after.Alloc - before.Alloc
		}

		timeSamples = append(timeSamples, float64(st.elapsed))
		nSum += uint64(n)
		prevN = int64(n)
		prevElapsed = st.elapsed
		st.alive = nil
		retried = false
	}

	unaccountedCPUTime := false
	if st.requiredSafety.Contains(starlark.CPUSafe) {
		if nSum < 1000 {
			// Very slow functions (e.g. ~1ms per N) can be problematic on
			// their own, even if they don't grow much with their inputs.
			unaccountedCPUTime = true
		} else {
			timeSamples = removeNoise(timeSamples)
			for i := 1; i < len(timeSamples); i++ {
				maxNegligibleElapsed := timeSamples[0] * math.Log(float64(samplingInterval*int64(i+1)+1))
				if timeSamples[i] > maxNegligibleElapsed {
					unaccountedCPUTime = true
				}
			}
		}
	}

	if valueTrackerAllocs > allocSum {
		allocSum = 0
	} else {
		allocSum -= valueTrackerAllocs
	}

	return executionStats{
		nSum:               nSum,
		allocSum:           allocSum,
		unaccountedCPUTime: unaccountedCPUTime,
	}
}

func decimate(samples []float64) []float64 {
	for i := 0; i < len(samples)/2; i++ {
		samples[i] = samples[i*2+1]
	}
	return samples[:len(samples)/2]
}

func log2_64(n uint64) int {
	if n == 0 {
		return 0
	}
	return 63 - bits.LeadingZeros64(n)
}

// removeNoise returns the given signal without spikes.
func removeNoise(signal []float64) []float64 {
	// The main idea is that monotonic functions (such as log(n), n^k, e^n)
	// will appear to have a low frequency compared with measurement noise
	// caused for example by interactions with the OS.
	//
	// To remove high frequency noise from the signal, this function uses an
	// IIR filter as simpler methods such as a moving average do not remove
	// enough noise to reliably determine function growth.
	filter := filterIIR{
		// Butterworth weights for 1/32 of the sampling frequency
		B: [3]float64{
			8.442692929079947e-03,
			1.688538585815989e-02,
			8.442692929079947e-03,
		},
		A: [3]float64{
			1,
			-1.723776172762509,
			0.757546944478829,
		},
	}
	return filter.BatchFilter(signal)
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
