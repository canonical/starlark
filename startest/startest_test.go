package startest_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	starlarktime "github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestRunBuiltin(t *testing.T) {
	st := startest.From(t)
	arg := starlark.String("test")
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

			if len(args) != 1 || args[0] != arg {
				st.Error("Wrong arguments received")
			}

			if len(kwargs) != 1 || len(kwargs[0]) != 2 || kwargs[0][0] != arg || kwargs[0][1] != arg {
				st.Error("Wrong kw arguments received")
			}

			return starlark.None, nil
		},
	)

	st.RunBuiltin(
		builtin,
		starlark.Tuple{arg},
		[]starlark.Tuple{{arg, arg}},
	)
}

func TestTrack(t *testing.T) {
	st := startest.From(t)

	// Check for a non-allocating routine
	st.RunThread(func(t *starlark.Thread, sd starlark.StringDict) {
		for i := 0; i < st.N; i++ {
			st.Track(nil)
		}
	})

	// Check for exact measuring
	st.RunThread(func(t *starlark.Thread, sd starlark.StringDict) {
		for i := 0; i < st.N; i++ {
			st.Track(new(int32))
			t.AddAllocs(4)
		}
	})

	// Check for over eximations
	st.RunThread(func(t *starlark.Thread, sd starlark.StringDict) {
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

func TestMakeArgs(t *testing.T) {
	// TODO: complete me!
}

func TestMakeKwargs(t *testing.T) {
	// TODO: complete me!
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
		t.Run(fmt.Sprintf("input-type=%v", test.from), func(t *testing.T) {
			st := startest.From(t)

			converted := st.ToValue(test.from)
			if !reflect.DeepEqual(converted, test.to) {
				t.Errorf("Incorrect starlark value conversion: expected %v but got %v", test.to, converted)
			}
		})
	}
}
