package startest_test

import (
	"testing"

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
