package starlark_test

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
)

type allocationTest struct {
	name           string
	gen            func(n uint) (program string, predecls env)
	trend          allocationTrend
	nSmall, nLarge uint
}
type env map[string]interface{}

const allocationErrorMargin = 0.1

// Tests allocations follow the specified trend, within a margin of error
func (test allocationTest) Run(t *testing.T) {
	if test.nSmall == 0 {
		test.nSmall = 1000
	}
	if test.nLarge == 0 {
		test.nLarge = 100000
	}

	deltaSmall, deltaLarge, err := test.computeAllocationDeltas()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	test.testAllocationAmount(t, test.nLarge, deltaLarge)

	test.testAllocationTrend(t, deltaSmall, deltaLarge)
}

func (test *allocationTest) computeAllocationDeltas() (deltaSmall, deltaLarge uint64, err error) {
	deltaSmall, err = test.computeMemoryIncrease(test.nSmall)
	if err != nil {
		return
	}
	deltaLarge, err = test.computeMemoryIncrease(test.nLarge)
	return
}

// Compute allocation delta declared when executing given code
func (test *allocationTest) computeMemoryIncrease(instanceSize uint) (uint64, error) {
	code, env := test.gen(instanceSize)
	predeclared, err := env.ToStarlarkPredecls()
	if err != nil {
		return 0, err
	}

	thread := new(starlark.Thread)
	_, err = starlark.ExecFile(thread, test.name, code, predeclared)
	return thread.Allocs(), err
}

// Test that expected number of allocations have been made, within a margin of error
func (test *allocationTest) testAllocationAmount(t *testing.T, n uint, delta uint64) {
	// Compute ratio between actual and expected
	expectedAllocs := test.trend.trend(float64(n))
	allocRatio := float64(delta) / expectedAllocs

	// Test ratio is within acceptable bounds
	if allocRatio < 1-allocationErrorMargin {
		t.Errorf("%s: too few allocations, expected ~%.0f but used only %d", test.name, expectedAllocs, delta)
	}
	if 1+allocationErrorMargin < allocRatio {
		t.Errorf("%s: too many allocations, expected ~%.0f but used %d", test.name, expectedAllocs, delta)
	}
}

// Test that the allocations made followed the expected trend
func (test *allocationTest) testAllocationTrend(t *testing.T, deltaSmall, deltaLarge uint64) {
	// Compute ratio of the observed trend and expected trend
	instanceDeltaRatio := float64(deltaLarge) / float64(deltaSmall)
	expectedRatio := test.trend.trend(float64(test.nLarge)) / test.trend.trend(float64(test.nSmall))
	ratioActualAgainstExpected := instanceDeltaRatio / expectedRatio

	// Test ratio is within acceptable bounds
	if ratioActualAgainstExpected <= 1-allocationErrorMargin || 1+allocationErrorMargin <= ratioActualAgainstExpected {
		t.Errorf("%s: memory allocations did not %s: f(%d)=%d, f(%d)=%d, ratio=%.3f, want ~%.0f", test.name, test.trend.label, test.nSmall, deltaSmall, test.nLarge, deltaLarge, instanceDeltaRatio, expectedRatio)
	}
}

// Convert an env to a starlark.StringDict for use as predeclared values when executing Starlark code.
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
	type envToPredeclsTest struct {
		from env
		to   starlark.StringDict
	}

	tests := []envToPredeclsTest{
		{
			from: env{},
			to:   starlark.StringDict{},
		},
		{
			from: env{"foo": "bar"},
			to:   starlark.StringDict{"foo": starlark.String("bar")},
		},
		{
			from: env{"foo": []string{"bar", "baz"}},
			to:   starlark.StringDict{"foo": starlark.NewList([]starlark.Value{starlark.String("bar"), starlark.String("baz")})},
		},
	}

	for _, test := range tests {
		if converted, err := test.from.ToStarlarkPredecls(); err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(converted, test.to) {
			t.Errorf("Incorrect starlark value conversion: expected %v (%T) but got %v (%T)", test.to, test.to, converted, converted)
		}
	}
}

// Convert some useful types to starlark values to make tests more readible
func toStarlarkValue(in interface{}) (starlark.Value, error) {
	// Special behaviours
	if in, ok := in.(starlark.Value); ok {
		return in, nil
	}
	if c, ok := in.(rune); ok { // Avoid considering this as just a regular int32
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
	case reflect.Pointer:
		return toStarlarkValue(inVal.Elem())
	case reflect.String:
		return starlark.String(inVal.String()), nil
	default:
		return nil, fmt.Errorf("Cannot automatically convert a value of kind %v to a starlark.Value: encountered %v", kind, in)
	}
}

func TestToStarlarkValue(t *testing.T) {
	type valueConversionTest struct {
		from interface{}
		to   starlark.Value
	}

	fooBarString := "foobar"

	tests := []valueConversionTest{
		{from: starlark.MakeInt(1234), to: starlark.MakeInt(1234)},
		{from: nil, to: starlark.None},
		{from: true, to: starlark.Bool(true)},
		{from: -1, to: starlark.MakeInt(-1)},
		{from: 'a', to: starlark.String("a")},
		{from: "bar", to: starlark.String("bar")},
		{from: byte(10), to: starlark.MakeInt(10)},
		{from: int(10), to: starlark.MakeInt(10)},
		{from: int8(10), to: starlark.MakeInt(10)},
		{from: int16(10), to: starlark.MakeInt(10)},
		// {from: int32(10), to: starlark.MakeInt(10)},
		{from: int64(10), to: starlark.MakeInt(10)},
		{from: uint(10), to: starlark.MakeInt(10)},
		{from: uint8(10), to: starlark.MakeInt(10)},
		{from: uint16(10), to: starlark.MakeInt(10)},
		{from: uint32(10), to: starlark.MakeInt(10)},
		{from: uint64(10), to: starlark.MakeInt(10)},
		{from: uintptr(10), to: starlark.MakeInt(10)},
		{from: float32(2.5), to: starlark.Float(2.5)},
		{from: float64(3.14), to: starlark.Float(3.14)},
		{from: &fooBarString, to: starlark.String(fooBarString)},
		{
			from: []string{"foo", "bar"},
			to:   starlark.NewList(append(make([]starlark.Value, 0), starlark.String("foo"), starlark.String("bar"))),
		},
		{
			from: [...]string{"foo", "bar"},
			to:   starlark.NewList(append(make([]starlark.Value, 0), starlark.String("foo"), starlark.String("bar"))),
		},
		{
			from: map[string]string{"foo": "bar"},
			to: func() starlark.Value {
				dict := starlark.NewDict(1)
				dict.SetKey(starlark.String("foo"), starlark.String("bar"))
				return dict
			}(),
		},
		{
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

type allocationTrend struct {
	label string
	trend func(n float64) float64
}

func constant(c float64) allocationTrend {
	return allocationTrend{
		label: "remain constant",
		trend: func(_ float64) float64 { return c },
	}
}

func TestConstantTrend(t *testing.T) {
	const expectedConstant = 104.0

	testValues := []float64{1, 2, 3, 4, 5, 6, 7}
	constTrend := constant(expectedConstant)
	for _, v := range testValues {
		if actual := constTrend.trend(v); actual != expectedConstant {
			t.Errorf("Constant trend did not remain constant: expected %g but got %g", expectedConstant, actual)
			break
		}
	}
}

func linear(a float64) allocationTrend {
	return allocationTrend{
		label: "increase linearly",
		trend: func(n float64) float64 { return a * n },
	}
}

func TestLinearTrend(t *testing.T) {
	const expectedGradient = 104.0

	// Compute example trend
	testValues := []float64{1, 2, 3, 4, 5, 6, 7}
	linearTrend := linear(expectedGradient)
	trendValues := make([]float64, len(testValues))
	for i, v := range testValues {
		trendValues[i] = linearTrend.trend(v)
	}

	// Check trend holds
	for i := 1; i < len(testValues); i++ {
		actualGradient := (trendValues[i] - trendValues[i-1]) / (testValues[i] - testValues[i-1])
		if actualGradient != expectedGradient {
			t.Errorf("Linear trend had incorrect gradient: expected %g but got %g", expectedGradient, actualGradient)
			break
		}
	}
}

func affine(a, b float64) allocationTrend {
	return allocationTrend{
		label: "increase affinely",
		trend: func(n float64) float64 { return a*n + b },
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
		trendValues[i] = affineTrend.trend(v)
	}

	// Check trend holds
	if actualIntercept := affineTrend.trend(0); actualIntercept != expectedIntercept {
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

func TestStringCapitalizeAllocations(t *testing.T) {
	allocationTest{
		name: "string.capitalize",
		gen: func(n uint) (string, env) {
			return "s.capitalize()", env{"s": dummyString(n, 's')}
		},
		trend: linear(1),
	}.Run(t)
}

func dummyString(len uint, char rune) string {
	return strings.Repeat(string(char), int(len))
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
