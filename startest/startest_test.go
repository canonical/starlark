package startest_test

import (
	"reflect"
	"testing"
	"time"

	starlarktime "github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestRunBuiltin(t *testing.T) {
	st := startest.From(t)

	expectedArgs := starlark.Tuple{starlark.String("a"), starlark.String("b"), starlark.String("c")}
	k := starlark.String("k")
	v := starlark.String("v")

	var builtin *starlark.Builtin
	builtin = starlark.NewBuiltin(
		"test",
		func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if fn != builtin {
				st.Error("Wrong builtin")
			}

			if thread == nil {
				st.Error("Thread is not available")
			}

			if !reflect.DeepEqual(args, expectedArgs) {
				st.Errorf("Incorrect arguments: expected %v but got %v", expectedArgs, args)
			}

			if len(kwargs) != 1 || len(kwargs[0]) != 2 || kwargs[0][0] != k || kwargs[0][1] != v {
				st.Error("Wrong kw arguments received")
			}

			return starlark.None, nil
		},
	)

	st.SetArgs(expectedArgs...)
	st.SetKwargs(starlark.StringDict{string(k): v})
	st.RunBuiltin(builtin)
}

func TestTrack(t *testing.T) {
	st := startest.From(t)

	// Check for a non-allocating routine
	st.SetMaxAllocs(0)
	st.RunThread(func(_ *starlark.Thread, _ starlark.StringDict) {
		for i := 0; i < st.N; i++ {
			st.Track(nil)
		}
	})

	// Check for exact measuring
	st.SetMaxAllocs(4)
	st.RunThread(func(t *starlark.Thread, _ starlark.StringDict) {
		for i := 0; i < st.N; i++ {
			st.Track(new(int32))
			t.AddAllocs(4)
		}
	})

	// Check for over estimations
	st.SetMaxAllocs(4)
	st.RunThread(func(t *starlark.Thread, _ starlark.StringDict) {
		for i := 0; i < st.N; i++ {
			st.Track(new(int32))
			t.AddAllocs(20)
		}
	})
}

func TestThread(t *testing.T) {
	st := startest.From(t)

	testBuiltin := starlark.NewBuiltin("testBuiltin", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	testValue := starlark.String("value")

	st.AddBuiltin(testBuiltin)

	st.AddValue("testValue", testValue)

	st.RunThread(func(thread *starlark.Thread, sd starlark.StringDict) {
		if sd == nil {
			st.Error("Received a nil environment")
		}

		if thread == nil {
			st.Error("Received a nil thread")
		}

		if v, ok := sd["testBuiltin"]; !ok {
			st.Error("testBuiltin not found")
		} else if v != testBuiltin {
			st.Error("wrong value expected")
		}

		if v, ok := sd["testValue"]; !ok {
			st.Error("testValue not found")
		} else if v != testValue {
			st.Error("wrong value expected")
		}
	})
}

func TestFailed(t *testing.T) {
	dummyT := &testing.T{}

	st := startest.From(dummyT)

	if st.Failed() {
		t.Error("Startest reported that it failed prematurely")
	}

	st.Log("foobar")

	if st.Failed() {
		t.Error("Startest reported that it failed prematurely")
	}

	st.Error("snafu")

	if !st.Failed() {
		t.Error("Startest did not report that it had failed")
	}
}

func TestValueConversion(t *testing.T) {
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
			to:   starlark.NewList([]starlark.Value{starlark.String("foo"), starlark.String("bar")}),
		}, {
			from: [...]string{"foo", "bar"},
			to:   starlark.NewList([]starlark.Value{starlark.String("foo"), starlark.String("bar")}),
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
				dict.SetKey(starlark.String("foo"), starlark.NewList([]starlark.Value{starlark.String("bar"), starlark.String("baz")}))
				return dict
			}(),
		}, {
			from: []starlark.String{starlark.String("foo"), starlark.String("bar")},
			to:   starlark.NewList([]starlark.Value{starlark.String("foo"), starlark.String("bar")}),
		},
	}

	for _, base := range []startest.TestBase{&testing.T{}, &testing.B{}, c} {
		for _, test := range tests {
			st := startest.From(base)

			converted := st.ToValue(test.from)
			if !reflect.DeepEqual(converted, test.to) {
				c.Errorf("Incorrect starlark value conversion: expected %v but got %v", test.to, converted)
			}

			if st.Failed() {
				c.Errorf("Unexpected failure converting values")
			}
		}
	}
}
