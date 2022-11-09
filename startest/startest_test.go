package startest_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	starlarktime "github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type checkSuite struct{}

var _ = check.Suite(&checkSuite{})

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

	st.AddArgs(expectedArgs[0])
	st.AddArgs(expectedArgs[1:]...)
	st.AddKwarg(string(k), v)
	st.RunBuiltin(builtin)
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

func (*checkSuite) TestFailed(c *check.C) {
	for _, base := range []startest.TestBase{&testing.T{}, &testing.B{}, c} {
		st := startest.From(base)

		if st.Failed() {
			c.Error("startest reported that it failed prematurely")
		}

		st.Log("foobar")

		if st.Failed() {
			c.Error("startest reported that it failed prematurely")
		}

		st.Error("snafu")

		if st.Failed() {
			c.Succeed()
		} else {
			c.Error("startest did not report that it had failed")
		}
	}
}

// func TestMakeArgs(t *testing.T) {
// 	st := startest.From(t)
// 	if emptyArgs := st.MakeArgs(); len(emptyArgs) != 0 {
// 		t.Errorf("empty MakeArgs call did not return empty args: got %v", emptyArgs)
// 	}

// 	value := starlark.String("value")
// 	if args := st.MakeArgs(value); len(args) != 1 {
// 		t.Errorf("computed args list had wrong length: expected 1, but got %d: %v", len(args), args)
// 	} else if args[0] != value {
// 		t.Errorf("value was changed: expected %v but got %v", value, args[0])
// 	}

// 	expected := starlark.Tuple{starlark.MakeInt(1234), starlark.MakeInt(4321)}
// 	if args := st.MakeArgs(1234, 4321); !reflect.DeepEqual(args, expected) {
// 		t.Errorf("incorrect args returned: expected %v git got %v", expected, args)
// 	}

// 	dummyT := testing.T{}
// 	dummiedSt := startest.From(&dummyT)
// 	if v := dummiedSt.MakeArgs(func() {}); v != nil {
// 		t.Errorf("expected unconvertable call to return nil: got %v", v)
// 	} else if !dummiedSt.Failed() {
// 		t.Errorf("expected test to be failed")
// 	}
// }

// func TestMakeKwargs(t *testing.T) {
// 	st := startest.From(t)

// 	if emptyKwargs := st.MakeKwargs(make(map[string]interface{})); len(emptyKwargs) != 0 {
// 		t.Errorf("Expected kwargs from empty map be empty: got %v", emptyKwargs)
// 	}

// 	in := map[string]interface{}{"k1": "v1", "k2": "v2"}
// 	expected := []starlark.Tuple{
// 		{starlark.String("k1"), starlark.String("v1")},
// 		{starlark.String("k2"), starlark.String("v2")},
// 	}
// 	if actual := st.MakeKwargs(in); len(actual) != 2 {
// 		if !reflect.DeepEqual(expected[0], actual[0]) && !reflect.DeepEqual(expected[0], actual[1]) {
// 			t.Errorf("Incorrect kwargs: expected %v but got %v", expected, actual)
// 		} else if !reflect.DeepEqual(expected[1], actual[0]) && !reflect.DeepEqual(expected[1], actual[1]) {
// 			t.Errorf("Incorrect kwargs: expected %v but got %v", expected, actual)
// 		}
// 	}

// 	dummyT := testing.T{}
// 	dummiedSt := startest.From(&dummyT)
// 	badInput := map[string]interface{}{"k": func() {}}

// 	if v := dummiedSt.MakeKwargs(badInput); v != nil {
// 		t.Errorf("Kwargs did not return nil: got %v", v)
// 	} else if !dummiedSt.Failed() {
// 		t.Error("Expected test set to be failed")
// 	}
// }

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
