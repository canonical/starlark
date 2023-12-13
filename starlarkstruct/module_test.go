package starlarkstruct_test

import (
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
	"github.com/canonical/starlark/startest"
)

var makeModule = starlark.NewBuiltinWithSafety("make_module", starlarkstruct.MakeModuleSafety, starlarkstruct.MakeModule)

func TestMakeModuleAllocs(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		pairs := make([][2]starlark.Value, st.N)
		kwargs := make([]starlark.Tuple, st.N)
		for i := 0; i < st.N; i++ {
			key := starlark.String(fmt.Sprintf("%012d", i))
			if err := thread.AddAllocs(starlark.EstimateSize(key)); err != nil {
				st.Error(err)
			}
			pairs[i][0], pairs[i][1] = key, starlark.None
			kwargs[i] = pairs[i][:]
		}
		args := starlark.Tuple{starlark.String("module")}
		result, err := starlark.Call(thread, makeModule, args, kwargs)
		if err != nil {
			st.Error(err)
		}
		st.KeepAlive(result)
	})
}

func TestMakeModuleSteps(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
	st.SetMinExecutionSteps(1)
	st.SetMaxExecutionSteps(1)
	st.RunThread(func(thread *starlark.Thread) {
		pairs := make([][2]starlark.Value, st.N)
		kwargs := make([]starlark.Tuple, st.N)
		for i := 0; i < st.N; i++ {
			key := starlark.String(fmt.Sprintf("%012d", i))
			if err := thread.AddAllocs(starlark.EstimateSize(key)); err != nil {
				st.Error(err)
			}
			pairs[i][0], pairs[i][1] = key, starlark.None
			kwargs[i] = pairs[i][:]
		}
		args := starlark.Tuple{starlark.String("module")}
		result, err := starlark.Call(thread, makeModule, args, kwargs)
		if err != nil {
			st.Error(err)
		}
		st.KeepAlive(result)
	})
}
