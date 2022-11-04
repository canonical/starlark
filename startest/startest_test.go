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
