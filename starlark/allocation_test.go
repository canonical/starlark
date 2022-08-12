package starlark_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
)

type allocationTest struct {
	name           string
	gen            codeGenerator
	trend          allocationTrend
	nSmall, nLarge uint
}
type codeGenerator func(n uint) (program prog, predecls env)
type prog string
type env map[string]interface{}

const allocationErrorMargin = 0.1

func (at *allocationTest) InitDefaults() {
	if at.nSmall == 0 {
		at.nSmall = 1000
	}
	if at.nLarge == 0 {
		at.nLarge = 100000
	}
}

// Tests allocations follow the specified trend, within a margin of error
func (test allocationTest) Run(t *testing.T) {
	test.InitDefaults()

	deltaSmall, deltaLarge, err := test.computeAllocationDeltas()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// Test allocation increase order
	test.testAllocationAmount(t, test.nLarge, deltaLarge)
	test.testAllocationTrend(t, deltaSmall, deltaLarge)
}

func (test *allocationTest) computeAllocationDeltas() (deltaSmall, deltaLarge uintptr, err error) {
	deltaSmall, err = test.computeMemoryIncrease(test.nSmall)
	if err != nil {
		return
	}
	deltaLarge, err = test.computeMemoryIncrease(test.nLarge)
	return
}

// Compute allocation delta declared when executing given code
func (test *allocationTest) computeMemoryIncrease(instanceSize uint) (uintptr, error) {
	code, env := test.gen(instanceSize)
	predeclared, err := env.ToStarlarkPredecls()
	if err != nil {
		return 0, err
	}

	thread := new(starlark.Thread)
	allocs0 := thread.Allocations()
	_, err = starlark.ExecFile(thread, test.name, code, predeclared)
	return thread.Allocations() - allocs0, err
}

// Test that expected number of allocations have been made, within a margin of error
func (test *allocationTest) testAllocationAmount(t *testing.T, n uint, delta uintptr) {
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
func (test *allocationTest) testAllocationTrend(t *testing.T, deltaSmall, deltaLarge uintptr) {
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
func toStarlarkValue(in interface{}) (out starlark.Value, err error) {
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
		out = starlark.None
	case reflect.Bool:
		out = starlark.Bool(inVal.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		out = starlark.MakeInt(int(inVal.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		out = starlark.MakeInt(int(inVal.Uint()))
	case reflect.Float32, reflect.Float64:
		out = starlark.Float(inVal.Float())
	case reflect.Map:
		d := starlark.NewDict(inVal.Len())
		iter := inVal.MapRange()
		for iter.Next() {
			var k, v starlark.Value
			if k, err = toStarlarkValue(iter.Key()); err != nil {
				return
			}
			if v, err = toStarlarkValue(iter.Value()); err != nil {
				return
			}
			d.SetKey(k, v)
		}
		out = d
	case reflect.Pointer:
		out, err = toStarlarkValue(inVal.Elem())
	case reflect.String:
		out = starlark.String(inVal.String())
	case reflect.Interface:
		out, err = toStarlarkValue(inVal.Elem())
	default:
		err = fmt.Errorf("Cannot automatically convert a value of kind %v to a starlark.Value: encountered %v", kind, in)
	}
	return
}

func TestToStarlarkValue(t *testing.T) {
	type valueConversionTest struct {
		from interface{}
		to   starlark.Value
	}

	fooBarStringRaw := "foobar"
	fooBarDict := starlark.NewDict(1)
	fooBarDict.SetKey(starlark.String("foo"), starlark.String("bar"))

	tests := []valueConversionTest{
		{
			from: starlark.String("foo"),
			to:   starlark.String("foo"),
		},
		{
			from: nil,
			to:   starlark.None,
		},
		{
			from: true,
			to:   starlark.Bool(true),
		},
		{
			from: -1,
			to:   starlark.MakeInt(-1),
		},
		{
			from: 'a',
			to:   starlark.String(string("a")),
		},
		{
			from: "bar",
			to:   starlark.String("bar"),
		},
		{
			from: uint(10),
			to:   starlark.MakeInt(10),
		},
		{
			from: float64(3.14),
			to:   starlark.Float(3.14),
		},
		{
			from: float32(2.5),
			to:   starlark.Float(2.5),
		},
		{
			from: map[string]string{"foo": "bar"},
			to:   fooBarDict,
		},
		{
			from: &fooBarStringRaw,
			to:   starlark.String(fooBarStringRaw),
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
		gen: func(n uint) (prog, env) {
			return "s.capitalize()", env{"s": dummyString(n, 's')}
		},
		trend: linear(1),
	}.Run(t)
}

func dummyString(len uint, char rune) string {
	return strings.Repeat(string(char), int(len))
}
