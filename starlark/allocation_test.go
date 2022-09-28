package starlark_test

import (
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	starlarktime "github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
)

type allocTest struct {
	name           string
	gen            func(n uint) (program string, predecls env)
	nSmall, nLarge uint
	trend
}
type env map[string]interface{}

const errorFraction = 0.1

// Run tests whether allocs follow the specified trend
func (test allocTest) Run(t *testing.T) {
	if test.nSmall == 0 {
		test.nSmall = 1000
	}
	if test.nLarge == 0 {
		test.nLarge = 100000
	}

	deltaSmall, err := test.computeDelta(test.nSmall)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	deltaLarge, err := test.computeDelta(test.nLarge)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	test.testAllocAmount(t, test.nLarge, deltaLarge)
	test.testAllocTrend(t, deltaSmall, deltaLarge)
}

func (test *allocTest) computeDelta(n uint) (uint64, error) {
	code, env := test.gen(n)
	predeclared, err := env.ToStarlarkPredecls()
	if err != nil {
		return 0, err
	}

	thread := new(starlark.Thread)
	_, err = starlark.ExecFile(thread, test.name, code, predeclared)
	return thread.Allocs(), err
}

// Test that expected number of allocs have been made, within a margin of error
func (test *allocTest) testAllocAmount(t *testing.T, n uint, delta uint64) {
	expectedDelta := test.Trend(float64(n))
	deltaRatio := float64(delta) / expectedDelta

	if deltaRatio < 1-errorFraction {
		t.Errorf("%s: too few allocations, expected ~%.0f but used only %d", test.name, expectedDelta, delta)
	} else if 1+errorFraction < deltaRatio {
		t.Errorf("%s: too many allocations, expected ~%.0f but used %d", test.name, expectedDelta, delta)
	}
}

// Test that the allocs made followed the expected trend
func (test *allocTest) testAllocTrend(t *testing.T, deltaSmall, deltaLarge uint64) {
	observedScaling := float64(deltaLarge) / float64(deltaSmall)
	expectedScaling := test.Trend(float64(test.nLarge)) / test.Trend(float64(test.nSmall))
	scalingRatio := observedScaling / expectedScaling

	if scalingRatio <= 1-errorFraction || 1+errorFraction <= scalingRatio {
		t.Errorf("%s: allocations did not %s: f(%d)=%d, f(%d)=%d, ratio=%.3f, want ~%.0f", test.name, test.trend.desc, test.nSmall, deltaSmall, test.nLarge, deltaLarge, observedScaling, expectedScaling)
	}
}

// ToStarlarkPredecls converts an env to a starlark.StringDict
func (e env) ToStarlarkPredecls() (starlark.StringDict, error) {
	predecls := make(starlark.StringDict, len(e))
	for key, val := range e {
		if v, err := toStarlarkValue(val); err != nil {
			return nil, err
		} else {
			predecls[key] = v
		}
	}
	return predecls, nil
}

func TestToStarlarkPredecls(t *testing.T) {
	type envTest struct {
		from env
		to   starlark.StringDict
	}

	tests := []envTest{{
		from: env{},
		to:   starlark.StringDict{},
	}, {
		from: env{"foo": "bar"},
		to:   starlark.StringDict{"foo": starlark.String("bar")},
	}, {
		from: env{"foo": []string{"bar", "baz"}},
		to:   starlark.StringDict{"foo": starlark.NewList([]starlark.Value{starlark.String("bar"), starlark.String("baz")})},
	}}

	for _, test := range tests {
		if converted, err := test.from.ToStarlarkPredecls(); err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(converted, test.to) {
			t.Errorf("Incorrect starlark value conversion: expected %v (%T) but got %v (%T)", test.to, test.to, converted, converted)
		}
	}
}

// toStarlarkValue converts go values to starlark ones. Handles arrays, slices,
// interfaces, maps and all scalar types except int32.
func toStarlarkValue(in interface{}) (starlark.Value, error) {
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
			if elems[i], err = toStarlarkValue(inVal.Index(i)); err != nil {
				return nil, err
			}
		}
		return starlark.NewList(elems), nil
	case reflect.Map:
		d := starlark.NewDict(inVal.Len())
		iter := inVal.MapRange()
		for iter.Next() {
			var k, v starlark.Value
			var err error
			if k, err = toStarlarkValue(iter.Key()); err != nil {
				return nil, err
			}
			if v, err = toStarlarkValue(iter.Value()); err != nil {
				return nil, err
			}
			d.SetKey(k, v)
		}
		return d, nil
	case reflect.String:
		return starlark.String(inVal.String()), nil
	default:
		return nil, fmt.Errorf("Cannot automatically convert a value of kind %v to a starlark.Value: encountered %v", kind, in)
	}
}

func TestToStarlarkValue(t *testing.T) {
	type conversionTest struct {
		from interface{}
		to   starlark.Value
	}

	value := starlarktime.Duration(time.Nanosecond)

	tests := []conversionTest{
		{from: value, to: value},
		{from: nil, to: starlark.None},
		{from: true, to: starlark.Bool(true)},
		{from: -1, to: starlark.MakeInt(-1)},
		{from: 'a', to: starlark.String("a")},
		{from: "bar", to: starlark.String("bar")},
		{from: rune(10), to: starlark.String("\n")},
		{from: byte(10), to: starlark.MakeInt(10)},
		{from: int(10), to: starlark.MakeInt(10)},
		{from: int8(10), to: starlark.MakeInt(10)},
		{from: int16(10), to: starlark.MakeInt(10)},
		{from: int64(10), to: starlark.MakeInt(10)},
		{from: uint(10), to: starlark.MakeInt(10)},
		{from: uint8(10), to: starlark.MakeInt(10)},
		{from: uint16(10), to: starlark.MakeInt(10)},
		{from: uint32(10), to: starlark.MakeInt(10)},
		{from: uint64(10), to: starlark.MakeInt(10)},
		{from: uintptr(10), to: starlark.MakeInt(10)},
		{from: float32(2.5), to: starlark.Float(2.5)},
		{from: float64(3.14), to: starlark.Float(3.14)},
		{
			from: []string{"foo", "bar"},
			to:   starlark.NewList(append(make([]starlark.Value, 0), starlark.String("foo"), starlark.String("bar"))),
		}, {
			from: [...]string{"foo", "bar"},
			to:   starlark.NewList(append(make([]starlark.Value, 0), starlark.String("foo"), starlark.String("bar"))),
		}, {
			from: map[string]string{"foo": "bar"},
			to: func() starlark.Value {
				dict := starlark.NewDict(1)
				dict.SetKey(starlark.String("foo"), starlark.String("bar"))
				return dict
			}(),
		}, {
			from: map[string][]string{"foo": {"bar", "baz"}},
			to: func() starlark.Value {
				dict := starlark.NewDict(1)
				dict.SetKey(starlark.String("foo"), starlark.NewList(append(make([]starlark.Value, 0, 2), starlark.String("bar"), starlark.String("baz"))))
				return dict
			}(),
		},
	}

	for _, test := range tests {
		if converted, err := toStarlarkValue(test.from); err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(converted, test.to) {
			t.Errorf("Incorrect starlark value conversion: expected %v but got %v", test.to, converted)
		}
	}
}

type trend struct {
	desc  string
	Trend func(n float64) float64
}

func constant(c float64) trend {
	return trend{
		desc:  "remain constant",
		Trend: func(_ float64) float64 { return c },
	}
}

func TestConstantTrend(t *testing.T) {
	const expected = 104.0

	testValues := []float64{1, 2, 3, 4, 5, 6, 7}
	constTrend := constant(expected)
	for _, v := range testValues {
		if actual := constTrend.Trend(v); actual != expected {
			t.Errorf("Constant trend did not remain constant: expected %g but got %g", expected, actual)
			break
		}
	}
}

func linear(a float64) trend {
	return trend{
		desc:  "increase linearly",
		Trend: func(n float64) float64 { return a * n },
	}
}

func TestLinearTrend(t *testing.T) {
	const expectedGradient = 104.0

	// Compute example trend
	inputs := []float64{1, 2, 3, 4, 5, 6, 7}
	linearTrend := linear(expectedGradient)
	outputs := make([]float64, len(inputs))
	for i, v := range inputs {
		outputs[i] = linearTrend.Trend(v)
	}

	for i := 1; i < len(inputs); i++ {
		actualGradient := (outputs[i] - outputs[i-1]) / (inputs[i] - inputs[i-1])
		if actualGradient != expectedGradient {
			t.Errorf("Linear trend had incorrect gradient: expected %g but got %g", expectedGradient, actualGradient)
			break
		}
	}
}

func affine(a, b float64) trend {
	return trend{
		desc:  "increase affinely",
		Trend: func(n float64) float64 { return a*n + b },
	}
}

func TestAffineTrend(t *testing.T) {
	const expectedGradient = 104.0
	const expectedIntercept = 1.0

	// Compute example trend
	testValues := []float64{1, 2, 3, 4, 5, 6, 7}
	affineTrend := affine(expectedGradient, expectedIntercept)
	trendValues := make([]float64, len(testValues))
	for i, v := range testValues {
		trendValues[i] = affineTrend.Trend(v)
	}

	if actualIntercept := affineTrend.Trend(0); actualIntercept != expectedIntercept {
		t.Errorf("Affine trend had incorrect value at zero: expected %g but got %g", expectedIntercept, actualIntercept)
	}
	for i := 1; i < len(testValues); i++ {
		actualGradient := (trendValues[i] - trendValues[i-1]) / (testValues[i] - testValues[i-1])
		if actualGradient != expectedGradient {
			t.Errorf("Affine trend had incorrect gradient: expected %g but got %g", expectedGradient, actualGradient)
			break
		}
	}
}

func dummyInt(len uint) starlark.Int {
	i := starlark.MakeInt(1)
	i = i.Lsh(len - 1)
	return i
}

func TestDummyInt(t *testing.T) {
	const expectedLen = 1024
	i := dummyInt(expectedLen)
	if actualLen := bits.UintSize * len(i.BigInt().Bits()); actualLen != expectedLen {
		t.Errorf("Incorrect dummy int length: expected %v but got %v", expectedLen, actualLen)
	}
}

func dummyString(len uint, char rune) starlark.String {
	return starlark.String(strings.Repeat(string(char), int(len)))
}

func TestDummyString(t *testing.T) {
	const expectedLen = 100
	dummy := string(dummyString(expectedLen, 'q'))

	if ok, err := regexp.MatchString("q*", dummy); err != nil {
		t.Error(err)
	} else if !ok {
		t.Errorf("Dummy string did not consist of the same character")
	}

	if actualLen := len(dummy); actualLen != expectedLen {
		t.Errorf("Dummy string had wrong length: expected %d but got %d", expectedLen, actualLen)
		t.Errorf("%s", dummy)
	}
}

func dummyBytes(len uint, char rune) starlark.Bytes {
	return starlark.Bytes(strings.Repeat(string(char), int(len)))
}

func TestDummyBytes(t *testing.T) {
	const expectedLen = 100
	dummy := string(dummyBytes(expectedLen, 'q'))

	if ok, err := regexp.MatchString("q*", dummy); err != nil {
		t.Error(err)
	} else if !ok {
		t.Errorf("Dummy string did not consist of the same character")
	}

	if actualLen := len(dummy); actualLen != expectedLen {
		t.Errorf("Dummy string had wrong length: expected %d but got %d", expectedLen, actualLen)
		t.Errorf("%s", dummy)
	}
}

func dummyList(len uint, elem interface{}) *starlark.List {
	elems := make([]starlark.Value, 0, len)
	elemValue, err := toStarlarkValue(elem)
	if err != nil {
		panic(err)
	}
	for i := uint(0); i < len; i++ {
		elems = append(elems, elemValue)
	}
	return starlark.NewList(elems)
}

func TestDummyList(t *testing.T) {
	const expectedLen = 1000
	const expectedElem = 'a'

	list := dummyList(expectedLen, expectedElem)
	if actualLen := list.Len(); actualLen != expectedLen {
		t.Errorf("Incorrect length: expectec %d but got %d", expectedLen, actualLen)
	}

	for i := 0; i < list.Len(); i++ {
		actualElem := list.Index(i)
		if elemStr, ok := actualElem.(starlark.String); !ok || string(elemStr) != string(expectedElem) {
			t.Errorf("Incorrect value stored: expected %v (%T) but got %v (%T)", expectedElem, expectedElem, actualElem, actualElem)
			break
		}
	}
}

func dummySet(len uint, first int) *starlark.Set {
	set := starlark.NewSet(int(len))
	for i := 0; i < int(len); i++ {
		set.Insert(starlark.MakeInt(first + i))
	}
	return set
}

func TestDummySet(t *testing.T) {
	const expectedLen = 1000
	const minElem = 25

	set := dummySet(expectedLen, minElem)

	if actualLen := set.Len(); actualLen != expectedLen {
		t.Errorf("Incorrect length: expected %d but got %d", expectedLen, actualLen)
	}

	for i := minElem; i < expectedLen+minElem; i++ {
		if ok, err := set.Has(starlark.MakeInt(i)); err != nil {
			t.Error(err)
		} else if !ok {
			t.Errorf("Expected %d to be in set", i)
			break
		}
	}
}

func dummyDict(len uint) *starlark.Dict {
	dict := starlark.NewDict(int(len))
	for i := 1; i <= int(len); i++ {
		s := starlark.String(strconv.Itoa(i))
		dict.SetKey("_"+s, s)
	}
	return dict
}

func TestDummyDict(t *testing.T) {
	const expectedLen = 1000
	dict := dummyDict(expectedLen)

	if actualLen := dict.Len(); actualLen != expectedLen {
		t.Errorf("Incorrect size: expected %d but got %d", expectedLen, actualLen)
	}

	elems := make(map[starlark.Value]struct{}, expectedLen)
	iter := dict.Iterate()
	defer iter.Done()

	var val starlark.Value
	for iter.Next(&val) {
		if _, ok := elems[val]; ok {
			t.Errorf("Duplicate element: %v", val)
			break
		}
		elems[val] = struct{}{}
	}
}

func TestDefaultAllocMaxIsUnbounded(t *testing.T) {
	thread := &starlark.Thread{}

	if err := thread.CheckAllocs(1); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if _, err := starlark.ExecFile(thread, "default_allocs_test", "", nil); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := thread.AddAllocs(math.MaxInt64); err != nil {
			t.Errorf("Unexpected error: %v", err)
			break
		}
	}
}

func TestCheckAllocs(t *testing.T) {
	thread := new(starlark.Thread)
	thread.SetMaxAllocs(1000)

	if err := thread.CheckAllocs(500); err != nil {
		t.Errorf("Unexpected error: %v", err)
	} else if allocs := thread.Allocs(); allocs != 0 {
		t.Errorf("CheckAllocs recorded allocations: expected 0 but got %v", allocs)
	}

	if err := thread.CheckAllocs(2000); err == nil {
		t.Errorf("Expected error")
	} else if err.Error() != "exceeded memory allocation limits" {
		t.Errorf("Unexpected error: %v", err)
	} else if allocs := thread.Allocs(); allocs != 0 {
		t.Errorf("CheckAllocs recorded allocations: expected 0 but got %v", allocs)
	}

	if _, err := starlark.ExecFile(thread, "alloc_cancel_test", "", nil); err != nil {
		t.Errorf("Unexpected cancellation: %v", err)
	}
}

func TestAllocDeclAndCheckBoundary(t *testing.T) {
	const allocCap = 1000
	thread := new(starlark.Thread)
	thread.SetMaxAllocs(allocCap)

	if err := thread.CheckAllocs(allocCap); err != nil {
		t.Errorf("Unexpected error: %v", err)
	} else if err := thread.CheckAllocs(allocCap + 1); err == nil {
		t.Errorf("Expected error checking too-many allocations")
	}

	if err := thread.AddAllocs(allocCap); err != nil {
		t.Errorf("Could not allocate entire quota: %v", err)
	} else {
		thread.AddAllocs(-allocCap)
		if err := thread.AddAllocs(allocCap + 1); err == nil {
			t.Errorf("Expected error when exceeding quota")
		}
	}
}

func TestPositiveDeltaDeclaration(t *testing.T) {
	const intendedAllocIncrease = 1000

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	// Accept and correctly store reasonable size increase
	allocs0 := thread.Allocs()
	if err := thread.AddAllocs(intendedAllocIncrease); err != nil {
		t.Errorf("Unexpected cancellation: %v", err)
	}
	delta := thread.Allocs() - allocs0
	if delta != intendedAllocIncrease {
		t.Errorf("Incorrect size increase: expected %d but got %d", intendedAllocIncrease, delta)
	}

	if _, err := starlark.ExecFile(thread, "alloc_cancel_test", "", nil); err != nil {
		t.Errorf("Unexpected cancellation: %v", err)
	}
}

func TestPositiveDeltaDeclarationExceedingMax(t *testing.T) {
	const allocationIncrease = 1000
	const maxAllocs = allocationIncrease / 2

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(maxAllocs)

	// Error when too much memory is required
	if err := thread.AddAllocs(allocationIncrease); err == nil {
		t.Errorf("Expected allocation failure!")
	}

	if allocs := thread.Allocs(); allocs != allocationIncrease {
		t.Errorf("Extra allocations were not recorded on an allocation failure: expected %d but %d were recorded", allocationIncrease, allocs)
	}

	if _, err := starlark.ExecFile(thread, "alloc_cancel_test", "", nil); err == nil {
		t.Errorf("Expected cancellation")
	} else if err.Error() != "Starlark computation cancelled: exceeded memory allocation limits" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestOverflowingPositiveDeltaDeclaration(t *testing.T) {
	const allocationIncrease = math.MaxInt64
	const expectedErrMsg = "exceeded memory allocation limits"

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	// Increase so that the next allocation will cause an overflow
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("Unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("Unexpected error when declaring allocation increase: %v", err)
	}

	// Check overflow detected
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("Unexpected error when overflowing allocations: %v", err)
	} else if allocs := thread.Allocs(); allocs != math.MaxUint64 {
		t.Errorf("Incorrect allocations stored: expected %d but got %d", uint64(math.MaxUint64), allocs)
	}

	// Check repeated overflow
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("Unexpected error when repeatedly overflowing allocations: %v", err)
	} else if allocs := thread.Allocs(); allocs != math.MaxUint64 {
		t.Errorf("Incorrect allocations stored: expected %d but got %d", uint64(math.MaxUint64), allocs)
	}
}

func TestNegativeDeltaDeclaration(t *testing.T) {
	const allocGreatest = 1000
	const allocReduction = 100
	const expectedFinalAllocs = allocGreatest - allocReduction

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	if err := thread.AddAllocs(allocGreatest); err != nil {
		t.Errorf("Unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(-allocReduction); err != nil {
		t.Errorf("Unexpected error when declaring allocation reduction: %v", err)
	}
	if allocs := thread.Allocs(); allocs != expectedFinalAllocs {
		t.Errorf("Increase and reduction of allocations lead to incorrect value: expected %v but got %v", expectedFinalAllocs, allocs)
	}
}

func TestOverzealousNegativeDeltaDeclaration(t *testing.T) {
	const allocGreatest = 1000
	const allocReduction = 2 * allocGreatest
	const expectedFinalAllocs = 0

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	if err := thread.AddAllocs(allocGreatest); err != nil {
		t.Errorf("Unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(-allocReduction); err != nil {
		t.Errorf("Unexpected error when declaring allocation reduction: %v", err)
	}
	if allocs := thread.Allocs(); allocs != 0 {
		t.Errorf("Expected overzealous alloc reduction to cap allocations at zero: recorded %d allocs instead", allocs)
	}
}

func TestConcurrentCheckAllocsUsage(t *testing.T) {
	const allocPeak = math.MaxUint64 ^ (math.MaxUint64 >> 1)
	const maxAllocs = allocPeak + 1
	const repetitions = 1_000_000

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(maxAllocs)
	thread.AddAllocs(allocPeak - 1)

	done := make(chan struct{}, 2)

	go func() {
		// Flip between 1000...00 and 0111...11 allocations
		for i := 0; i < repetitions; i++ {
			thread.AddAllocs(1)
			thread.AddAllocs(-1)
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < repetitions; i++ {
			// Check 1000...01 not exceeded
			if err := thread.CheckAllocs(1); err != nil {
				t.Errorf("Unexpected error: %v", err)
				break
			}
		}
		done <- struct{}{}
	}()

	// Await goroutine completion
	totDone := 0
	for totDone != 2 {
		select {
		case <-done:
			totDone++
		}
	}
}

func TestConcurrentAddAllocsUsage(t *testing.T) {
	const expectedAllocs = 1_000_000

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	done := make(chan struct{}, 2)

	callAddAlloc := func(n uint) {
		for i := uint(0); i < n; i++ {
			thread.AddAllocs(1)
		}
		done <- struct{}{}
	}

	go callAddAlloc(expectedAllocs / 2)
	go callAddAlloc(expectedAllocs / 2)

	// Await goroutine completion
	totDone := 0
	for totDone != 2 {
		select {
		case <-done:
			totDone++
		}
	}

	if allocs := thread.Allocs(); allocs != expectedAllocs {
		t.Errorf("Concurrent thread.AddAlloc contains a race, expected %d allocs recorded but got %d", expectedAllocs, allocs)
	}
}

func TestCheckAllocsCancelledRejection(t *testing.T) {
	const cancellationReason = "arbitrary cancellation reason"
	const maxAllocs = 1000

	thread := new(starlark.Thread)
	thread.Cancel(cancellationReason)
	thread.SetMaxAllocs(maxAllocs)

	if err := thread.CheckAllocs(2 * maxAllocs); err == nil {
		t.Errorf("Expected cancellation")
	} else if err.Error() != cancellationReason {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestAddAllocsCancelledRejection(t *testing.T) {
	const cancellationReason = "arbitrary cancellation reason"
	const maxAllocs = 1000

	thread := new(starlark.Thread)
	thread.Cancel(cancellationReason)
	thread.SetMaxAllocs(maxAllocs)

	if err := thread.AddAllocs(2 * maxAllocs); err == nil {
		t.Errorf("Expected cancellation")
	} else if err.Error() != cancellationReason {
		t.Errorf("Unexpected error: %v", err)
	} else if allocs := thread.Allocs(); allocs != 0 {
		t.Errorf("Changes were recorded against cancelled thread: expected 0 allocations, got %v", allocs)
	}
}

func TestIntAllocations(t *testing.T) {
	test := allocTest{
		name: "int-builtin",
		gen: func(n uint) (program string, predecl env) {
			number := strings.Repeat("deadbeef", int(n))

			return `
x = int(number, 16)
`, env{"number": number}
		},
		trend: linear(4),
	}

	test.Run(t)
}
