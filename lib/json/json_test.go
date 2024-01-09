package json_test

import (
	"errors"
	"fmt"
	"strings"
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
	json_decode, _ := json.Module.Attr("decode")
	if json_decode == nil {
		t.Fatal("no such method: json.decode")
	}

	const populatedLength = 1000
	tests := []struct {
		name              string
		input             string
		minExecutionSteps uint64
		maxExecutionSteps uint64
	}{{
		name:  "int",
		input: "48879",
	}, {
		name:  "big-int",
		input: "3825590844416",
	}, {
		name:  "float",
		input: "1.4218e-1",
	}, {
		name:  "bool",
		input: "true",
	}, {
		name:  "bool",
		input: "true",
	}, {
		name:  "null",
		input: "null",
	}, {
		name:              "string",
		input:             `"tnetennba"`,
		minExecutionSteps: 10,
		maxExecutionSteps: 10,
	}, {
		name:  "empty-list",
		input: "[]",
	}, {
		name: "populated-list",
		input: func() string {
			buf := &strings.Builder{}
			buf.WriteByte('[')
			for i := 0; i < populatedLength; i++ {
				if i > 0 {
					buf.WriteString(", ")
				}
				buf.WriteByte('1')
			}
			buf.WriteByte(']')
			return buf.String()
		}(),
		minExecutionSteps: populatedLength,
		maxExecutionSteps: populatedLength,
	}, {
		name:              "nested-list",
		input:             "[[[]]]",
		minExecutionSteps: 2,
		maxExecutionSteps: 2,
	}, {
		name:  "empty-mapping",
		input: "{}",
	}, {
		name: "populated-mapping",
		input: func() string {
			buf := &strings.Builder{}
			buf.WriteByte('{')
			for i := 0; i < populatedLength; i++ {
				if i > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(fmt.Sprintf(`"%10d": %d`, i, i))
			}
			buf.WriteByte('}')
			return buf.String()
		}(),
		minExecutionSteps: (2 + 11) * populatedLength, // Expect on average 2.5*len steps for insertion, 11 per parsed key
		maxExecutionSteps: (3 + 11) * populatedLength,
	}, {
		name:              "nested-mapping",
		input:             `{"l1": {"l2": {"l3": {}}}}`,
		minExecutionSteps: 3 + 3*3, // 3 steps for the nesting, 3 steps per parsed key
		maxExecutionSteps: 3 + 3*3,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe)
			st.SetMinExecutionSteps(test.minExecutionSteps)
			st.SetMaxExecutionSteps(test.maxExecutionSteps)
			st.RunThread(func(thread *starlark.Thread) {
				json_document := starlark.String(test.input)
				for i := 0; i < st.N; i++ {
					_, err := starlark.Call(thread, json_decode, starlark.Tuple{json_document}, nil)
					if err != nil {
						st.Error(err)
					}
				}
			})
		})
	}
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
			"Tuple": [ 1, 2 ],
			"String": "tnetennba"
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
