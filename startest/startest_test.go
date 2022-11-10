package startest_test

import (
	"reflect"
	"testing"

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
