package startest_test

import (
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestExpectError(t *testing.T) {
}

func TestMaxAllocs(t *testing.T) {
}

func TestRunBuiltin(t *testing.T) {
	st := startest.From(t)
	arg := starlark.String("test")
	st.SetMaxAllocs(1)
	st.RunBuiltin(
		starlark.NewBuiltin(
			"test",
			func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				if len(args) != 1 || args[0] != arg {
					st.Error("Wrong arguments received")
				}

				if len(kwargs) != 1 || len(kwargs[0]) != 2 || kwargs[0][0] != arg || kwargs[0][1] != arg {
					st.Error("Wrong kw arguments received")
				}

				thread.AddAllocs(1)
				return starlark.None, nil
			},
		),
		starlark.Tuple{arg},
		[]starlark.Tuple{{arg, arg}},
	)
}

func TestRunThread(t *testing.T) {
}

func TestTrack(t *testing.T) {
	st := startest.From(t)
	st.SetMaxAllocs(0)
	st.RunThread(func(t *starlark.Thread, sd starlark.StringDict) {
		for i := 0; i < st.N; i++ {
			st.Track(nil)
		}
	})
}

func TestPredeclared(t *testing.T) {
	st := startest.From(t)
	testBuiltin := starlark.NewBuiltin("testBuiltin", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	testValue := starlark.String("value")

	st.AddBuiltin(testBuiltin)

	st.AddValue("testValue", testValue)

	st.RunThread(func(t *starlark.Thread, sd starlark.StringDict) {
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
