package startest

import (
	"math"
	"reflect"
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

type starTest struct {
	predefined    starlark.StringDict
	builtinArgs   starlark.Tuple
	builtinKwargs []starlark.Tuple
	maxAllocs     uint64
	margin        float64
	tracked       []interface{}
	N             int
	testBase
}

var _ testBase = &testing.T{}
var _ testBase = &testing.B{}
var _ testBase = &check.C{}

// From returns a new starTest instance with a given test base.
func From(base testBase) *starTest {
	return &starTest{testBase: base, maxAllocs: math.MaxUint64, margin: 0.05}
}

// AddBuiltin inserts the given Builtin into the predeclared values passed to
// RunThread.
func (test *starTest) AddBuiltin(fn *starlark.Builtin) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[fn.Name()] = fn
}

// AddValue inserts the given value into the predeclared values passed to
// RunThread.
func (test *starTest) AddValue(name string, value starlark.Value) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[name] = value
}

// AddArgs allows the given values to be passed as arguments to a RunBuiltin call.
func (test *starTest) SetArgs(args ...starlark.Value) {
	test.builtinArgs = args
}

// AddArgs allows the given key-value pair to be passed as keyword-arguments to
// a RunBuiltin call.
func (test *starTest) SetKwargs(kwargs starlark.StringDict) {
	test.builtinKwargs = make([]starlark.Tuple, 0, len(kwargs))

	for key, value := range kwargs {
		test.builtinKwargs = append(test.builtinKwargs, starlark.Tuple{starlark.String(key), value})
	}
}

// SetMaxAllocs optionally sets the max allocations allowed per test.N
func (test *starTest) SetMaxAllocs(maxAllocs uint64) {
	test.maxAllocs = maxAllocs
}

// SetRealityMargin sets the fraction by which measured allocations can be greater
// than from declared allocations
func (test *starTest) SetRealityMargin(margin float64) {
	if test.margin > 0 {
		test.margin = margin
	} else {
		test.margin = 0
	}
}

// RunBuiltin tests the given builtin
func (test *starTest) RunBuiltin(fn starlark.Value) {
	if _, ok := fn.(*starlark.Builtin); !ok {
		test.Error("fn must be a builtin")
		return
	}

	test.RunThread(func(th *starlark.Thread, globals starlark.StringDict) {
		for i := 0; i < test.N; i++ {
			result, err := starlark.Call(th, fn, test.builtinArgs, test.builtinKwargs)
			if err != nil {
				test.Error(err)
			}

			test.Track(result)
		}
	})
}

// RunThread tests a function which has access to a starlark thread and a global environment
func (test *starTest) RunThread(fn func(*starlark.Thread, starlark.StringDict)) {
	thread := &starlark.Thread{}
	thread.SetMaxAllocs(test.maxAllocs)

	meanMeasured, nTotal := test.measureMemory(func() {
		fn(thread, test.predefined)
	})

	if test.Failed() {
		return
	}

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

// Track forces the memory usage of the given objects to be tracked
func (test *starTest) Track(v ...interface{}) {
	test.tracked = append(test.tracked, v...)
}

func (test *starTest) measureMemory(fn func()) (meanMemory, nTotal uint64) {
	defer func() { test.tracked = make([]interface{}, 0) }()

	startNano := time.Now().Nanosecond()

	const nMax = 100_000
	const memoryTarget = 100 * 2 << 20
	const timeMax = 1e9

	var memoryUsed uint64
	var valueTrackerOverhead uint64
	test.N = 0
	nTotal = 0

	for n := uint64(0); !test.Failed() && memoryUsed-valueTrackerOverhead < memoryTarget && n < nMax && (time.Now().Nanosecond()-startNano) < timeMax; {
		last := n
		prevIters := uint64(test.N)
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
		nTotal += n

		var before, after runtime.MemStats
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)

		fn()

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&after)

		iterationMeasure := int64(after.Alloc - before.Alloc)
		if iterationMeasure > 0 {
			memoryUsed += uint64(iterationMeasure)
		}
		valueTrackerOverhead = uint64(cap(test.tracked)) * uint64(unsafe.Sizeof(interface{}(nil)))
	}

	if test.Failed() {
		return 0, 1
	}

	memoryUsed -= valueTrackerOverhead

	return uint64(memoryUsed) / nTotal, nTotal
}

// ToValue converts go values to starlark ones. Handles arrays, slices,
// interfaces, maps and all scalar types except int32.
func (test *starTest) ToValue(in interface{}) starlark.Value {
	if test.Failed() {
		return nil
	}

	// Special behaviours
	if in, ok := in.(starlark.Value); ok {
		return in
	}
	if c, ok := in.(rune); ok {
		return starlark.String(c)
	}

	var inVal reflect.Value
	if v, ok := in.(reflect.Value); ok {
		inVal = v
	} else {
		inVal = reflect.ValueOf(in)
	}

	kind := inVal.Kind()
	switch kind {
	case reflect.Invalid:
		return starlark.None
	case reflect.Bool:
		return starlark.Bool(inVal.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt(int(inVal.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return starlark.MakeInt(int(inVal.Uint()))
	case reflect.Float32, reflect.Float64:
		return starlark.Float(inVal.Float())
	case reflect.Array, reflect.Slice:
		len := inVal.Len()
		elems := make([]starlark.Value, len)
		for i := 0; i < len; i++ {
			elems[i] = test.ToValue(inVal.Index(i))
			if test.Failed() {
				return nil
			}
		}
		return starlark.NewList(elems)
	case reflect.Map:
		d := starlark.NewDict(inVal.Len())
		iter := inVal.MapRange()
		for iter.Next() {
			k := test.ToValue(iter.Key())
			v := test.ToValue(iter.Value())
			d.SetKey(k, v)
		}
		return d
	case reflect.String:
		return starlark.String(inVal.String())
	case reflect.Interface:
		return test.ToValue(inVal.Interface())
	case reflect.Ptr:
		return test.ToValue(inVal.Elem())
	default:
		test.Errorf("Cannot automatically convert a value of kind %v to a starlark.Value: encountered %v", kind, in)
		return nil
	}
}
