package startest

import (
	"fmt"
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

func From(base testBase) *starTest {
	return &starTest{testBase: base, maxAllocs: math.MaxUint64, margin: 0.05}
}

func (test *starTest) AddBuiltin(fn *starlark.Builtin) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[fn.Name()] = fn
}

func (test *starTest) AddValue(name string, value starlark.Value) {
	if test.predefined == nil {
		test.predefined = make(starlark.StringDict)
	}
	test.predefined[name] = value
}

func (test *starTest) SetMaxAllocs(maxAllocs uint64) {
	test.maxAllocs = maxAllocs
}

func (test *starTest) SetMargin(margin float64) {
	test.margin = margin
}

func (test *starTest) RunBuiltin(fn starlark.Value, args starlark.Tuple, kwargs []starlark.Tuple) {
	if _, ok := fn.(*starlark.Builtin); ok {
		test.RunThread(func(th *starlark.Thread, globals starlark.StringDict) {
			for i := 0; i < test.N; i++ {
				result, err := starlark.Call(th, fn, args, kwargs)
				if err != nil {
					test.Error(err)
				}

				test.Track(result)
			}
		})
	} else {
		test.Error("fn must be a builtin")
	}
}

func (test *starTest) RunThread(fn func(*starlark.Thread, starlark.StringDict)) {
	thread := &starlark.Thread{}
	thread.SetMaxAllocs(test.maxAllocs)

	meanMeasured := test.measureMemory(func() {
		fn(thread, test.predefined)
	})

	if test.Failed() {
		return
	}

	if meanMeasured > test.maxAllocs {
		test.Errorf("measured memory is above maximum (%d > %d)", meanMeasured, test.maxAllocs)
	}

	if thread.Allocs() > test.maxAllocs {
		test.Errorf("thread allocations are above maximum (%d > %d)", meanMeasured, test.maxAllocs)
	}

	meanAllocs := (thread.Allocs() * uint64((1+test.margin)*100) / 100) / uint64(test.N)

	if meanMeasured > meanAllocs {
		test.Errorf("mean measured memory is more than 5%% above thread allocations (%d > %d)", meanMeasured, meanAllocs)
	}
}

func (test *starTest) Track(v ...interface{}) {
	test.tracked = append(test.tracked, v...)
}

func (test *starTest) measureMemory(fn func()) uint64 {
	defer func() { test.tracked = make([]interface{}, 0) }()

	startNano := time.Now().Nanosecond()

	const nMax = 100_000
	const memoryTarget = 100 * 2 << 20
	const timeMax = 1e9

	var memoryUsed int64
	var valueTrackerOverhead int64
	test.N = 0
	nTotal := int64(0)

	for n := int64(0); !test.Failed() && memoryUsed-valueTrackerOverhead < memoryTarget && n < nMax && (time.Now().Nanosecond()-startNano) < timeMax; {
		last := n
		prevIters := int64(test.N)
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
			memoryUsed += iterationMeasure
		}
		valueTrackerOverhead = int64(cap(test.tracked)) * int64(unsafe.Sizeof(interface{}(nil)))
	}

	if test.Failed() {
		return 0
	}

	memoryUsed -= valueTrackerOverhead

	return uint64(memoryUsed / nTotal)
}

func (test *starTest) MakeArgs(raw ...interface{}) (starlark.Tuple, error) {
	args := make(starlark.Tuple, len(raw))

	for _, rawArg := range raw {
		if arg, err := ToValue(rawArg); err == nil {
			args = append(args, arg)
		} else {
			return nil, err
		}
	}
	return args, nil
}

func (test *starTest) MakeKwargs(raw map[string]interface{}) ([]starlark.Tuple, error) {
	kwargs := make([]starlark.Tuple, len(raw))

	for k, v := range raw {
		k, err := ToValue(k)
		if err != nil {
			return nil, err
		}

		v, err := ToValue(v)
		if err != nil {
			return nil, err
		}

		kwargs = append(kwargs, starlark.Tuple{k, v})
	}

	return kwargs, nil
}

// ToValue converts go values to starlark ones. Handles arrays, slices,
// interfaces, maps and all scalar types except int32.
func ToValue(in interface{}) (starlark.Value, error) {
	// Special behaviours
	if in, ok := in.(starlark.Value); ok {
		return in, nil
	}
	if c, ok := in.(rune); ok {
		return starlark.String(c), nil
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
		return starlark.None, nil
	case reflect.Bool:
		return starlark.Bool(inVal.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt(int(inVal.Int())), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return starlark.MakeInt(int(inVal.Uint())), nil
	case reflect.Float32, reflect.Float64:
		return starlark.Float(inVal.Float()), nil
	case reflect.Array, reflect.Slice:
		len := inVal.Len()
		elems := make([]starlark.Value, len)
		for i := 0; i < len; i++ {
			var err error
			if elems[i], err = ToValue(inVal.Index(i)); err != nil {
				return nil, err
			}
		}
		return starlark.NewList(elems), nil
	case reflect.Map:
		d := starlark.NewDict(inVal.Len())
		iter := inVal.MapRange()
		for iter.Next() {
			k, err := ToValue(iter.Key())
			if err != nil {
				return nil, err
			}
			v, err2 := ToValue(iter.Value())
			if err2 != nil {
				return nil, err2
			}
			d.SetKey(k, v)
		}
		return d, nil
	case reflect.String:
		return starlark.String(inVal.String()), nil
	case reflect.Interface:
		return ToValue(inVal.Interface())
	case reflect.Ptr:
		return ToValue(inVal.Elem())
	default:
		return nil, fmt.Errorf("Cannot automatically convert a value of kind %v to a starlark.Value: encountered %v", kind, in)
	}
}
