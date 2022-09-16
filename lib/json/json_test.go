package json_test

import (
	"testing"

	"github.com/canonical/starlark/lib/json"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestModuleSafeties(t *testing.T) {
	for name, value := range json.Module.Members {
		builtin, ok := value.(*starlark.Builtin)
		if !ok {
			continue
		}

		if safety, ok := (*json.Safeties)[name]; !ok {
			t.Errorf("builtin json.%s has no safety declaration", name)
		} else if actual := builtin.Safety(); actual != safety {
			t.Errorf("builtin json.%s has incorrect safety: expected %v but got %v", name, safety, actual)
		}
	}

	for name, _ := range *json.Safeties {
		if _, ok := json.Module.Members[name]; !ok {
			t.Errorf("safety declared for non-existent builtin json.%s", name)
		}
	}
}

func TestJsonEncodeAllocs(t *testing.T) {
	json_encode, _ := json.Module.Attr("encode")
	if json_encode == nil {
		t.Fatal("no such method: json.endoce")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		json_object := &starlark.Dict{}
		json_object.SetKey(starlark.String("Int"), starlark.MakeInt(0xbeef))
		json_object.SetKey(starlark.String("BigInt"), starlark.MakeInt64(0xdeadbeef<<10))
		json_object.SetKey(starlark.String("Float"), starlark.Float(1.4218e-1))
		json_object.SetKey(starlark.String("Bool"), starlark.True)
		json_object.SetKey(starlark.String("Null"), starlark.None)
		json_object.SetKey(starlark.String("Empty list"), starlark.NewList([]starlark.Value{}))
		json_object.SetKey(starlark.String("Tuple"), starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)})

		array := make(starlark.Tuple, st.N)
		for i := 0; i < st.N; i++ {
			array[i] = json_object
		}

		result, err := starlark.Call(thread, json_encode, starlark.Tuple{array}, nil)
		if err != nil {
			st.Error(err)
		}
		st.KeepAlive(result)
	})
}

func TestJsonDecodeAllocs(t *testing.T) {
}

func TestJsonIndentAllocs(t *testing.T) {
	st := startest.From(t)

	obj := starlark.String(`{"l":[[[[[[{"i":10,"n":null}]]]]]]}`)
	fn := json.Module.Members["indent"]
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, fn, starlark.Tuple{obj}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}
