package json_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/canonical/starlark/lib/json"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

type unsafeTestIterable struct {
	testBase startest.TestBase
}

var _ starlark.Iterable = &unsafeTestIterable{}

func (ui *unsafeTestIterable) Freeze() {}
func (ui *unsafeTestIterable) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", ui.Type())
}
func (ui *unsafeTestIterable) String() string       { return "unsafeTestIterable" }
func (ui *unsafeTestIterable) Truth() starlark.Bool { return false }
func (ui *unsafeTestIterable) Type() string         { return "unsafeTestIterable" }
func (ui *unsafeTestIterable) Iterate() starlark.Iterator {
	return &unsafeTestIterator{
		testBase: ui.testBase,
	}
}

type unsafeTestIterator struct {
	testBase startest.TestBase
}

var _ starlark.Iterator = &unsafeTestIterator{}

func (ui *unsafeTestIterator) Next(p *starlark.Value) bool {
	ui.testBase.Error("Next called")
	return false
}
func (ui *unsafeTestIterator) Done()      {}
func (ui *unsafeTestIterator) Err() error { return fmt.Errorf("Err called") }

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

func TestJsonEncodeSteps(t *testing.T) {
	json_encode, _ := json.Module.Attr("encode")
	if json_encode == nil {
		t.Fatal("no such method: json.encode")
	}

	t.Run("safety-respected", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.CPUSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, json_encode, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if !errors.Is(err, starlark.ErrSafety) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	tests := []struct {
		name  string
		input starlark.Value
		steps uint64
	}{{
		name:  "Int (small)",
		input: starlark.MakeInt(0xbeef),
		steps: uint64(len(fmt.Sprintf("%d", 0xbeef))),
	}, {
		name:  "Int (big)",
		input: starlark.MakeInt64(0xdeadbeef << 10),
		steps: uint64(len(fmt.Sprintf("%d", 0xdeadbeef<<10))),
	}, {
		name:  "Float",
		input: starlark.Float(1.4218e-1),
		steps: uint64(len("0.14218")),
	}, {
		name:  "Bool",
		input: starlark.True,
		steps: uint64(len("True")),
	}, {
		name:  "None",
		input: starlark.None,
		steps: uint64(len("null")),
	}, {
		name:  "String",
		input: starlark.String(`"tnetennba"`),
		steps: uint64(len(`"\"tnetennba\""`)),
	}, {
		name:  "Tuple",
		input: starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)},
		steps: uint64(len("[1,2]")) + 3,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("standalone", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe)
				st.SetMinExecutionSteps(test.steps)
				st.SetMaxExecutionSteps(test.steps)
				st.RunThread(func(thread *starlark.Thread) {
					for i := 0; i < st.N; i++ {
						_, err := starlark.Call(thread, json_encode, starlark.Tuple{test.input}, nil)
						if err != nil {
							st.Error(err)
						}
					}
				})
			})
			t.Run("list", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe)
				const listOverheadSteps = 2 // iteration + writing ','
				st.SetMinExecutionSteps(test.steps + listOverheadSteps)
				st.SetMaxExecutionSteps(test.steps + listOverheadSteps)
				st.RunThread(func(thread *starlark.Thread) {
					elems := make([]starlark.Value, st.N)
					for i := 0; i < st.N; i++ {
						elems[i] = test.input
					}
					_, err := starlark.Call(thread, json_encode, starlark.Tuple{starlark.NewList(elems)}, nil)
					if err != nil {
						st.Error(err)
					}
				})
			})

			t.Run("map", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe)
				const listOverheadSteps = 2 // iteration + writing ','
				st.SetMinExecutionSteps(test.steps + uint64(len(`,"000000000000":`)+1))
				st.SetMaxExecutionSteps(test.steps + uint64(len(`,"000000000000":`)+1))
				st.RunThread(func(thread *starlark.Thread) {
					dict := starlark.NewDict(st.N)
					for i := 0; i < st.N; i++ {
						dict.SetKey(starlark.String(fmt.Sprintf("%012d", i)), test.input)
					}
					_, err := starlark.Call(thread, json_encode, starlark.Tuple{dict}, nil)
					if err != nil {
						st.Error(err)
					}
				})
			})
		})
	}
}

func TestJsonEncodeAllocs(t *testing.T) {
	json_encode, _ := json.Module.Attr("encode")
	if json_encode == nil {
		t.Fatal("no such method: json.encode")
	}

	t.Run("safety-respected", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, json_encode, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if !errors.Is(err, starlark.ErrSafety) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("builtin-types", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			pairs := []struct {
				key   string
				value starlark.Value
			}{
				{"Int", starlark.MakeInt(0xbeef)},
				{"BigInt", starlark.MakeInt64(0xdeadbeef << 10)},
				{"Float", starlark.Float(1.4218e-1)},
				{"Bool", starlark.True},
				{"Null", starlark.None},
				{"Empty list", starlark.NewList([]starlark.Value{})},
				{"Tuple", starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)}},
			}
			dictToMarshal := &starlark.Dict{}
			for _, pair := range pairs {
				dictToMarshal.SetKey(starlark.String(pair.key), pair.value)
			}
			array := make(starlark.Tuple, st.N)
			for i := 0; i < st.N; i++ {
				array[i] = dictToMarshal
			}
			result, err := starlark.Call(thread, json_encode, starlark.Tuple{array}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestJsonDecodeSteps(t *testing.T) {
}

func TestJsonDecodeAllocs(t *testing.T) {
	json_decode, _ := json.Module.Attr("decode")
	if json_decode == nil {
		t.Fatal("no such method: json.decode")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		json_document := starlark.String(`
		{
			"Int": 48879,
			"BigInt": 3825590844416,
			"Float": 1.4218e-1,
			"Bool": true,
			"Null": null,
			"Empty list": [],
			"Tuple": [ 1, 2 ]	
		}`)

		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, json_decode, starlark.Tuple{json_document}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestJsonIndentSteps(t *testing.T) {
	indent, ok := json.Module.Members["indent"]
	if !ok {
		t.Fatal("no such builtin: json.indent")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
	// 127 is the lenght of the expected indented json.
	st.SetMinExecutionSteps(127)
	st.SetMaxExecutionSteps(127)
	st.RunThread(func(thread *starlark.Thread) {
		document := starlark.String(`{"l":[[[[[[{"i":10,"n":null}]]]]]]}`)
		for i := 0; i < st.N; i++ {
			_, err := starlark.Call(thread, indent, starlark.Tuple{document}, nil)
			if err != nil {
				st.Error(err)
			}
		}
	})
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
