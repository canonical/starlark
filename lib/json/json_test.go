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
	// Allows test errors to be declared in methods without error returns.
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
	// Allows test errors to be declared in methods without error returns.
	testBase startest.TestBase
}

var _ starlark.Iterator = &unsafeTestIterator{}

func (ui *unsafeTestIterator) Next(p *starlark.Value) bool {
	ui.testBase.Error("Next called")
	return false
}
func (ui *unsafeTestIterator) Done()      {}
func (ui *unsafeTestIterator) Err() error { return fmt.Errorf("Err called") }

// testIterable is an iterable with customisable yield behaviour.
type testIterable struct {
	// If positive, maxN sets an upper bound on the number of iterations
	// performed. Otherwise, iteration is unbounded.
	maxN int

	// nth returns a value to be yielded by the nth Next call.
	nth func(thread *starlark.Thread, n int) (starlark.Value, error)
}

var _ starlark.Iterable = &testIterable{}

func (ti *testIterable) Freeze() {}
func (ti *testIterable) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", ti.Type())
}
func (ti *testIterable) String() string       { return "testIterable" }
func (ti *testIterable) Truth() starlark.Bool { return ti.maxN != 0 }
func (ti *testIterable) Type() string         { return "testIterable" }
func (ti *testIterable) Iterate() starlark.Iterator {
	return &testIterator{
		maxN: ti.maxN,
		nth:  ti.nth,
	}
}

type testIterator struct {
	n, maxN int
	nth     func(thread *starlark.Thread, n int) (starlark.Value, error)
	thread  *starlark.Thread
	err     error
}

var _ starlark.SafeIterator = &testIterator{}

func (it *testIterator) BindThread(thread *starlark.Thread) { it.thread = thread }
func (it *testIterator) Safety() starlark.SafetyFlags {
	const safe = starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe | starlark.IOSafe
	if it.thread == nil {
		return starlark.NotSafe
	}
	return safe
}
func (it *testIterator) Next(p *starlark.Value) bool {
	it.n++
	if it.nth == nil {
		it.err = errors.New("testIterator called with nil nth function")
	}
	if it.err != nil {
		return false
	}

	if it.maxN > 0 && it.n > it.maxN {
		return false
	}
	ret, err := it.nth(it.thread, it.n)
	if err != nil {
		it.err = err
		return false
	}

	*p = ret
	return true
}
func (it *testIterator) Done()      {}
func (it *testIterator) Err() error { return it.err }

func isStarlarkCancellation(err error) bool {
	return strings.Contains(err.Error(), "Starlark computation cancelled:")
}

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
		steps: uint64(len(fmt.Sprintf("%d", int64(0xdeadbeef<<10)))),
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
		steps: uint64(len("[1,2]")) + 2,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("standalone", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe)
				st.SetMinSteps(test.steps)
				st.SetMaxSteps(test.steps)
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
				st.SetMinSteps(test.steps + listOverheadSteps)
				st.SetMaxSteps(test.steps + listOverheadSteps)
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
				st.SetMinSteps(test.steps + uint64(len(`,"000000000000":`)+1))
				st.SetMaxSteps(test.steps + uint64(len(`,"000000000000":`)+1))
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

func TestJsonEncodeCancellation(t *testing.T) {
	json_encode, _ := json.Module.Attr("encode")
	if json_encode == nil {
		t.Fatal("no such method: json.encode")
	}

	t.Run("safety-respected", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.TimeSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, json_encode, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if !errors.Is(err, starlark.ErrSafety) {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("iterable", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.TimeSafe)
		st.SetMaxSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			thread.Cancel("done")
			iter := &testIterable{
				maxN: st.N,
				nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
					return starlark.None, nil
				},
			}
			_, err := starlark.Call(thread, json_encode, starlark.Tuple{iter}, nil)
			if err == nil {
				st.Errorf("expected cancellation: %d", st.N)
			} else if !isStarlarkCancellation(err) {
				st.Errorf("expected cancellation, got: %v", err)
			}
		})
	})

	t.Run("mapping", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.TimeSafe)
		st.SetMaxSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			thread.Cancel("done")
			dictToMarshal := starlark.NewDict(st.N)
			for i := 0; i < st.N; i++ {
				dictToMarshal.SetKey(starlark.String(fmt.Sprint(i)), starlark.None)
			}
			_, err := starlark.Call(thread, json_encode, starlark.Tuple{dictToMarshal}, nil)
			if err == nil {
				st.Error("expected cancellation")
			} else if !isStarlarkCancellation(err) {
				st.Errorf("expected cancellation, got: %v", err)
			}
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
		name     string
		input    string
		minSteps uint64
		maxSteps uint64
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
		name:     "string",
		input:    `"tnetennba"`,
		minSteps: 10,
		maxSteps: 10,
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
		minSteps: populatedLength,
		maxSteps: populatedLength,
	}, {
		name:     "nested-list",
		input:    "[[[]]]",
		minSteps: 2,
		maxSteps: 2,
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
		minSteps: (2 + 11) * populatedLength, // Expect on average 2.5*len steps for insertion, 11 per parsed key
		maxSteps: (3 + 11) * populatedLength,
	}, {
		name:     "nested-mapping",
		input:    `{"l1": {"l2": {"l3": {}}}}`,
		minSteps: 3 + 3*3, // 3 steps for the nesting, 3 steps per parsed key
		maxSteps: 3 + 3*3,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe)
			st.SetMinSteps(test.minSteps)
			st.SetMaxSteps(test.maxSteps)
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

func TestJsonDecodeCancellation(t *testing.T) {
	json_decode, _ := json.Module.Attr("decode")
	if json_decode == nil {
		t.Fatal("no such method: json.decode")
	}

	const populatedLength = 1000
	tests := []struct {
		name     string
		input    string
		minSteps uint64
		maxSteps uint64
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
		name:     "string",
		input:    `"tnetennba"`,
		minSteps: 10,
		maxSteps: 10,
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
		minSteps: populatedLength,
		maxSteps: populatedLength,
	}, {
		name:     "nested-list",
		input:    "[[[]]]",
		minSteps: 2,
		maxSteps: 2,
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
		minSteps: (2 + 11) * populatedLength, // Expect on average 2.5*len steps for insertion, 11 per parsed key
		maxSteps: (3 + 11) * populatedLength,
	}, {
		name:     "nested-mapping",
		input:    `{"l1": {"l2": {"l3": {}}}}`,
		minSteps: 3 + 3*3, // 3 steps for the nesting, 3 steps per parsed key
		maxSteps: 3 + 3*3,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.TimeSafe)
			st.SetMaxSteps(0)
			st.RunThread(func(thread *starlark.Thread) {
				thread.Cancel("done")
				json_document := starlark.String("[" + strings.Repeat(test.input+",", st.N) + "null]")
				_, err := starlark.Call(thread, json_decode, starlark.Tuple{json_document}, nil)
				if err == nil {
					st.Error("expected cancellation")
				} else if !isStarlarkCancellation(err) {
					st.Errorf("expected cancellation, got: %v", err)
				}
			})
		})
	}

	t.Run("padded", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.TimeSafe)
		st.SetMaxSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			thread.Cancel("done")
			json_document := starlark.String(strings.Repeat(" ", st.N) + "1")
			_, err := starlark.Call(thread, json_decode, starlark.Tuple{json_document}, nil)
			if err == nil {
				st.Error("expected cancellation")
			} else if !isStarlarkCancellation(err) {
				st.Errorf("expected cancellation, got: %v", err)
			}
		})
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
	st.SetMinSteps(127)
	st.SetMaxSteps(127)
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
