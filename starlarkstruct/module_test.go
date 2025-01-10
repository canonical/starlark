package starlarkstruct_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
	"github.com/canonical/starlark/startest"
)

var makeModule = starlark.NewBuiltinWithSafety("make_module", starlarkstruct.MakeModuleSafety, starlarkstruct.MakeModule)

func TestModuleSafeString(t *testing.T) {
	module := &starlarkstruct.Module{
		Name: "foo",
		Members: starlark.StringDict{
			"bar": starlark.None,
		},
	}

	t.Run("nil-thread", func(t *testing.T) {
		defer func() {
			if err := recover(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()

		builder := new(strings.Builder)
		if err := module.SafeString(nil, builder); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("consistency", func(t *testing.T) {
		thread := &starlark.Thread{}
		builder := new(strings.Builder)
		if err := module.SafeString(thread, builder); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		unsafeResult := module.String()
		safeResult := builder.String()
		if unsafeResult != safeResult {
			t.Errorf("inconsistent stringer implementation: expected %s got %s", unsafeResult, safeResult)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.TimeSafe)
		st.SetMaxSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			thread.Cancel("done")
			builder := starlark.NewSafeStringBuilder(thread)
			err := module.SafeString(thread, builder)
			if err == nil {
				st.Error("expected cancellation")
			} else if !isStarlarkCancellation(err) {
				st.Errorf("expected cancellation, got: %v", err)
			}
		})
	})
}

func TestModuleSafeAttr(t *testing.T) {
	module := &starlarkstruct.Module{
		Name: "foo",
		Members: starlark.StringDict{
			"bar": starlark.None,
		},
	}

	t.Run("nil-thread", func(t *testing.T) {
		defer func() {
			if err := recover(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()

		_, err := module.SafeAttr(nil, "bar")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("consistency", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(starlarkstruct.MakeModuleSafety)

		safeResult, err := module.SafeAttr(nil, "bar")
		if err != nil {
			t.Error(err)
		}
		unsafeResult, err := module.Attr("bar")
		if err != nil {
			t.Error(err)
		}
		if safeResult != unsafeResult {
			t.Errorf("unconsistent SafeAttr implementation: expected %v and %v to be equal", safeResult, unsafeResult)
		}
	})
}

func TestMakeModuleResources(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
	st.SetMinSteps(1)
	st.SetMaxSteps(1)
	st.RunThread(func(thread *starlark.Thread) {
		pairs := make([][2]starlark.Value, st.N)
		kwargs := make([]starlark.Tuple, st.N)
		for i := 0; i < st.N; i++ {
			key := starlark.String(fmt.Sprintf("%012d", i))
			if err := thread.AddAllocs(starlark.EstimateSizeOld(key)); err != nil {
				st.Error(err)
			}
			pairs[i] = [2]starlark.Value{key, starlark.None}
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
