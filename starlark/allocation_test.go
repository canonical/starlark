package starlark_test

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	starlarktime "github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
)

type AllocTest struct {
	TestGenerator
	Ns                          []uint
	ErrorCoefficient            float64
	ReportedAllocsTrend         Trend
	MeasuredAllocsTrend         Trend
	MeasuredTotalAllocsTrend    Trend
	ReportedAllocsBounding      Bound
	MeasuredAllocsBounding      Bound
	MeasuredTotalAllocsBounding Bound
}
type Env map[string]interface{}

type TestGenerator interface {
	Name() string
	Setup(n uint) (interface{}, error)
	Run(t *testing.T, thread *starlark.Thread, pre interface{}) (interface{}, error)
	Measure(thread *starlark.Thread, pre, post interface{}) uint64
}

type Trend interface {
	Desc() string
	At(n float64) float64
}

type Bound int

const (
	Above Bound = 1 << iota
	Below
	Unbounded
)

// Run tests whether allocs follow the specified trend
func (test AllocTest) Run(t *testing.T) {
	if err := test.init(); err != nil {
		t.Error(err)
		return
	}

	reportedAllocs := make([]int64, len(test.Ns))
	measuredAllocs := make([]int64, len(test.Ns))
	measuredTotalAllocs := make([]int64, len(test.Ns))
	for i, n := range test.Ns {
		pre, err := test.Setup(n)
		if err != nil {
			t.Errorf("%s: Unexpected error during setup: %v", test.Name(), err)
			return
		}

		thread := &starlark.Thread{}

		var before, after runtime.MemStats

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)

		post, err := test.TestGenerator.Run(t, thread, pre)
		if err != nil {
			t.Errorf("%s: Unexpected error: %v", test.Name(), err)
			return
		}

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&after)

		runtime.KeepAlive(pre)
		runtime.KeepAlive(thread)
		runtime.KeepAlive(post)

		reportedAllocs[i] = int64(test.Measure(thread, pre, post))
		measuredAllocs[i] = int64(after.Alloc - before.Alloc)
		measuredTotalAllocs[i] = int64(after.TotalAlloc - before.TotalAlloc)
	}

	test.testTrend(t, "reported allocs", test.Ns, reportedAllocs, test.ReportedAllocsTrend, test.ReportedAllocsBounding)
	test.testTrend(t, "measured allocs", test.Ns, measuredAllocs, test.MeasuredAllocsTrend, test.MeasuredAllocsBounding)
	test.testTrend(t, "measured total allocs", test.Ns, measuredTotalAllocs, test.MeasuredTotalAllocsTrend, test.MeasuredTotalAllocsBounding)
}

func (test *AllocTest) init() error {
	if test.TestGenerator == nil {
		return errors.New("nil test generator")
	}
	if test.ReportedAllocsTrend == nil {
		return fmt.Errorf("%s: Reported allocs trend is nil", test.Name())
	}

	if test.Ns == nil {
		test.Ns = []uint{
			1000,
			10000,
			100000,
		}
	}
	if test.MeasuredAllocsTrend == nil {
		test.MeasuredAllocsTrend = test.ReportedAllocsTrend
	}
	if test.MeasuredTotalAllocsTrend == nil {
		test.MeasuredTotalAllocsTrend = test.ReportedAllocsTrend
	}
	if test.ErrorCoefficient == 0 {
		test.ErrorCoefficient = 0.1
	}

	if test.MeasuredAllocsBounding == 0 {
		test.MeasuredAllocsBounding = Above | Below
	}
	if test.ReportedAllocsBounding == 0 {
		test.ReportedAllocsBounding = Above | Below
	}
	if test.MeasuredTotalAllocsBounding == 0 {
		test.MeasuredTotalAllocsBounding = Above
	}

	if test.ErrorCoefficient <= 0 || 1 <= test.ErrorCoefficient {
		return fmt.Errorf("%s: invalid error coefficient: expected between 0 and 1 but got %f", test.Name(), test.ErrorCoefficient)
	}
	return nil
}

// testTrend checks that a trend was followed over a slice of instance sizes
// and measurements.
func (test AllocTest) testTrend(t *testing.T, measurementDesc string, ns []uint, measurements []int64, expectation Trend, bound Bound) {
	if bound&Unbounded != 0 {
		return
	}

	for i, n := range ns {
		expected := expectation.At(float64(n))
		measured := float64(measurements[i])

		tooMany := bound&Above != 0 && (1+test.ErrorCoefficient)*expected < measured
		tooFew := bound&Below != 0 && measured < (1-test.ErrorCoefficient)*expected
		if tooMany || tooFew {
			t.Errorf("%s: %s did not %s: for input sizes %v, observed %v", test.Name(), measurementDesc, expectation.Desc(), ns, measurements)
			break
		}
	}
}

type BuiltinGenerator struct {
	Builtin starlark.Value
	Recv    func(n uint) interface{}
	Args    func(n uint) []interface{}
	Kwargs  func(n uint) map[string]starlark.Value
}

var _ TestGenerator = &BuiltinGenerator{}

func (bt *BuiltinGenerator) Name() string {
	if bt.Builtin == nil {
		return "nil"
	}
	if b, ok := bt.Builtin.(*starlark.Builtin); ok {
		return b.Name()
	}
	return bt.Builtin.String()
}

type builtinCall struct {
	Builtin *starlark.Builtin
	Args    starlark.Tuple
	Kwargs  []starlark.Tuple
}

func (bt *BuiltinGenerator) Setup(n uint) (interface{}, error) {
	builtin, ok := bt.Builtin.(*starlark.Builtin)
	if !ok {
		if bt.Builtin == nil {
			return nil, fmt.Errorf("Expected a builtin, got nil")
		}
		return nil, fmt.Errorf("Expected a builtin, got a %v", bt.Builtin.Type())
	}

	if bt.Recv != nil {
		v, err := toStarlarkValue(bt.Recv(n))
		if err != nil {
			return nil, fmt.Errorf("Could not convert receiver: %v", err)
		}
		builtin = builtin.BindReceiver(v)
	}

	testCase := &builtinCall{Builtin: builtin}

	if bt.Args != nil {
		args := bt.Args(n)
		testCase.Args = make(starlark.Tuple, len(args))
		for i, arg := range args {
			v, err := toStarlarkValue(arg)
			if err != nil {
				return nil, fmt.Errorf("Could not convert arg %d: %v", i+1, err)
			}
			testCase.Args = append(testCase.Args, v)
		}
	}
	if bt.Kwargs != nil {
		kwargs := bt.Kwargs(n)
		testCase.Kwargs = make([]starlark.Tuple, len(kwargs))
		for k, v := range kwargs {
			v, err := toStarlarkValue(v)
			if err != nil {
				return nil, fmt.Errorf("Could not convert kwarg %s: %v", k, err)
			}
			pair := starlark.Tuple{starlark.String(k), v}
			testCase.Kwargs = append(testCase.Kwargs, pair)
		}
	}

	return testCase, nil
}

func (bt *BuiltinGenerator) Run(t *testing.T, thread *starlark.Thread, pre interface{}) (interface{}, error) {
	testCase := pre.(*builtinCall)
	return testCase.Builtin.CallInternal(thread, testCase.Args, testCase.Kwargs)
}

func (bt *BuiltinGenerator) Measure(thread *starlark.Thread, pre, post interface{}) uint64 {
	return thread.Allocs()
}

var _ TestGenerator = &BuiltinGenerator{}

// ToStarlarkPredecls converts an env to a starlark.StringDict
func (e Env) ToStarlarkPredecls() (starlark.StringDict, error) {
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
		from Env
		to   starlark.StringDict
	}

	tests := []envTest{{
		from: Env{},
		to:   starlark.StringDict{},
	}, {
		from: Env{"foo": "bar"},
		to:   starlark.StringDict{"foo": starlark.String("bar")},
	}, {
		from: Env{"foo": []string{"bar", "baz"}},
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
	case reflect.Interface:
		return toStarlarkValue(inVal.Interface())
	case reflect.Ptr:
		return toStarlarkValue(inVal.Elem())
	default:
		return nil, fmt.Errorf("Cannot automatically convert a value of kind %v to a starlark.Value: encountered %v", kind, in)
	}
}

func TestToStarlarkValue(t *testing.T) {
	type conversionTest struct {
		from interface{}
		to   starlark.Value
	}

	str := "foobar"
	strPtr := &str
	value := starlarktime.Duration(time.Nanosecond)

	tests := []conversionTest{
		{from: value, to: value},
		{from: nil, to: starlark.None},
		{from: true, to: starlark.Bool(true)},
		{from: -1, to: starlark.MakeInt(-1)},
		{from: 'a', to: starlark.String("a")},
		{from: str, to: starlark.String(str)},
		{from: &str, to: starlark.String(str)},
		{from: &strPtr, to: starlark.String(str)},
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
		}, {
			from: []starlark.String{starlark.String("foo"), starlark.String("bar")},
			to:   starlark.NewList(append(make([]starlark.Value, 0), starlark.String("foo"), starlark.String("bar"))),
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

type Constant struct{ constant float64 }

var _ Trend = Constant{}

func (c Constant) Desc() string         { return fmt.Sprintf("remain constant at %.3f", c.constant) }
func (c Constant) At(_ float64) float64 { return c.constant }

func TestConstantTrend(t *testing.T) {
	const expected = 104.0

	testValues := []float64{1, 2, 3, 4, 5, 6, 7}
	constTrend := Constant{expected}
	for _, v := range testValues {
		if actual := constTrend.At(v); actual != expected {
			t.Errorf("Constant trend did not remain constant: expected %g but got %g", expected, actual)
			break
		}
	}
}

type Linear struct{ gradient float64 }

var _ Trend = Linear{}

func (l Linear) Desc() string {
	return fmt.Sprintf("increase linearly with gradient=%.3f", l.gradient)
}
func (l Linear) At(n float64) float64 { return l.gradient * n }

func TestLinearTrend(t *testing.T) {
	const expectedGradient = 104.0

	// Compute example trend
	inputs := []float64{1, 2, 3, 4, 5, 6, 7}
	linearTrend := Linear{expectedGradient}
	outputs := make([]float64, len(inputs))
	for i, v := range inputs {
		outputs[i] = linearTrend.At(v)
	}

	for i := 1; i < len(inputs); i++ {
		actualGradient := (outputs[i] - outputs[i-1]) / (inputs[i] - inputs[i-1])
		if actualGradient != expectedGradient {
			t.Errorf("Linear trend had incorrect gradient: expected %g but got %g", expectedGradient, actualGradient)
			break
		}
	}
}

type Affine struct{ gradient, intercept float64 }

var _ Trend = Affine{}

func (a Affine) Desc() string {
	return fmt.Sprintf("increase affinely with gradient=%.3f, intercept=%.3f", a.gradient, a.intercept)
}
func (a Affine) At(n float64) float64 { return a.gradient*n + a.intercept }

func TestAffineTrend(t *testing.T) {
	const expectedGradient = 104.0
	const expectedIntercept = 1.0

	// Compute example trend
	testValues := []float64{1, 2, 3, 4, 5, 6, 7}
	affineTrend := Affine{expectedGradient, expectedIntercept}
	trendValues := make([]float64, len(testValues))
	for i, v := range testValues {
		trendValues[i] = affineTrend.At(v)
	}

	if actualIntercept := affineTrend.At(0); actualIntercept != expectedIntercept {
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

func dummyDict(len int) *starlark.Dict {
	dict := starlark.NewDict(len)
	for i := 1; i <= len; i++ {
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
