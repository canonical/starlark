package starlark_test

import (
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestUnaryAllocs(t *testing.T) {
	t.Run("not", func(t *testing.T) {
		inputs := []starlark.Value{
			starlark.None,
			starlark.True,
			starlark.Tuple{},
			starlark.MakeInt(1),
			starlark.Float(1),
			starlark.NewList(nil),
			starlark.NewDict(1),
			starlark.NewSet(1),
			starlark.String("1"),
			starlark.Bytes("1"),
		}
		for _, input := range inputs {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.AddValue("input", input)
			st.RunString(`
				for _ in st.ntimes():
					st.keep_alive(not input)
			`)
		}
	})

	t.Run("minus", func(t *testing.T) {
		inputs := []starlark.Value{
			starlark.MakeInt(10),
			starlark.MakeInt64(1 << 40),
		}
		for _, input := range inputs {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.AddValue("input", input)
			st.RunString(`
				i = input
				for _ in st.ntimes():
					i = -i
					st.keep_alive(i)
			`)
		}
	})

	t.Run("plus", func(t *testing.T) {
		inputs := []starlark.Value{
			starlark.MakeInt(10),
			starlark.MakeInt64(1 << 40),
		}
		for _, input := range inputs {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.AddValue("input", input)
			st.RunString(`
				i = input
				for _ in st.ntimes():
					i = +i
					st.keep_alive(i)
			`)
		}
	})

	t.Run("neg", func(t *testing.T) {
		inputs := []starlark.Value{
			starlark.MakeInt(10),
			starlark.MakeInt64(1 << 40),
		}
		for _, input := range inputs {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.AddValue("input", input)
			st.RunString(`
				i = input
				for _ in st.ntimes():
					i = ~i
					st.keep_alive(i)
			`)
		}
	})
}

func TestTupleCreation(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunString(`
		for _ in st.ntimes():
			st.keep_alive(())
	`)
	st.RunString(`
		for _ in st.ntimes():
			st.keep_alive((1, "2", 3.0))
	`)
}

func TestListCreation(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunString(`
		for _ in st.ntimes():
			st.keep_alive([])
	`)
	st.RunString(`
		for _ in st.ntimes():
			st.keep_alive([ False, 1, "2", 3.0 ])
	`)
}

func TestListComprehension(t *testing.T) {
	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			st.keep_alive([v for v in st.ntimes()])
		`)
	})

	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive([v for v in range(2)])
		`)
	})
}

func TestDictCreation(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunString(`
		for _ in st.ntimes():
			st.keep_alive({})
	`)
	st.RunString(`
		for _ in st.ntimes():
			st.keep_alive({ 1: False, 2: "2", 3: 3.0 })
	`)
}

func TestDictComprehension(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunString(`
		for _ in st.ntimes():
			st.keep_alive({i:i for i in range(10)})
	`)
	st.RunString(`
		st.keep_alive({i:i for i in range(st.n)})
	`)
}

func TestIterate(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunString(`
		for i in range(st.n):
			st.keep_alive(i)
	`)
	st.RunString(`
		for i in range(st.n):
			st.keep_alive(i)
			for j in range(2):
				st.keep_alive(j)
				break
	`)
}

func TestSequenceAssignment(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunString(`
		for _ in st.ntimes():
			[ single ] = range(1)
			st.keep_alive(single)
	`)
	st.RunString(`
		for _ in st.ntimes():
			[ first, second ] = range(2)
			st.keep_alive(first, second)
	`)
	st.RunString(`
		for _ in st.ntimes():
			first, second = range(2)
			st.keep_alive(first, second)
	`)
}

func TestAttrAccessAllocs(t *testing.T) {
	tests := []struct {
		name  string
		attr  string
		input starlark.Value
	}{{
		name:  "List",
		attr:  "append",
		input: starlark.NewList(nil),
	}, {
		name:  "Dict",
		attr:  "update",
		input: starlark.NewDict(1),
	}, {
		name:  "Set",
		attr:  "union",
		input: starlark.NewSet(1),
	}, {
		name:  "String",
		attr:  "capitalize",
		input: starlark.String("1"),
	}, {
		name:  "Bytes",
		attr:  "elems",
		input: starlark.Bytes("1"),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
			st.SetMinExecutionSteps(1)
			st.AddValue("input", test.input)
			st.RunString(fmt.Sprintf(`
				for _ in st.ntimes():
					st.keep_alive(input.%s)
			`, test.attr))
		})
	}
}

type unsafeTestIndexable struct{}

var _ starlark.Indexable = &unsafeTestIndexable{}
var _ starlark.HasSetIndex = &unsafeTestIndexable{}

func (uti *unsafeTestIndexable) Freeze()              {}
func (uti *unsafeTestIndexable) String() string       { return "unsafeTestIndexable" }
func (uti *unsafeTestIndexable) Truth() starlark.Bool { return false }
func (uti *unsafeTestIndexable) Type() string         { return "<unsafeTestIndexable>" }
func (uti *unsafeTestIndexable) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", uti.Type())
}
func (uti *unsafeTestIndexable) Len() int                 { return 1 }
func (uti *unsafeTestIndexable) Index(int) starlark.Value { panic("Index called") }
func (*unsafeTestIndexable) SetIndex(index int, v starlark.Value) error {
	return fmt.Errorf("SetIndex called")
}

type unsafeTestMapping struct{}

var _ starlark.Mapping = &unsafeTestMapping{}
var _ starlark.HasSetKey = &unsafeTestMapping{}

func (utm *unsafeTestMapping) Freeze()              {}
func (utm *unsafeTestMapping) String() string       { return "unsafeTestMapping" }
func (utm *unsafeTestMapping) Truth() starlark.Bool { return false }
func (utm *unsafeTestMapping) Type() string         { return "<unsafeTestMapping>" }
func (utm *unsafeTestMapping) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", utm.Type())
}
func (utm *unsafeTestMapping) Get(starlark.Value) (v starlark.Value, found bool, err error) {
	return nil, false, fmt.Errorf("unsafeTestMapping.Get called")
}
func (*unsafeTestMapping) SetKey(k starlark.Value, v starlark.Value) error {
	return fmt.Errorf("SetKey called")
}

func TestIndexing(t *testing.T) {
	t.Run("safety-respected", func(t *testing.T) {
		tests := []struct {
			name  string
			input starlark.Value
		}{{
			name:  "indexable",
			input: &unsafeTestIndexable{},
		}, {
			name:  "mapping",
			input: &unsafeTestMapping{},
		}}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Run("get", func(t *testing.T) {
					dummy := &testing.T{}
					st := startest.From(dummy)
					st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
					st.AddValue("input", test.input)
					ok := st.RunString(`
						input[0]
					`)
					if ok {
						st.Error("expected error")
					}
				})

				t.Run("set", func(t *testing.T) {
					dummy := &testing.T{}
					st := startest.From(dummy)
					st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
					st.AddValue("input", test.input)
					ok := st.RunString(`
						input[0] = None
					`)
					if ok {
						st.Error("expected error")
					}
				})
			})
		}
	})

	t.Run("builtin", func(t *testing.T) {
		t.Run("get", func(t *testing.T) {
			tests := []struct {
				name  string
				input starlark.Value
			}{{
				name:  "bytes",
				input: starlark.Bytes("test"),
			}, {
				name: "dict",
				input: func() starlark.Value {
					dict := starlark.NewDict(1)
					dict.SetKey(starlark.MakeInt(0), starlark.None)
					return dict
				}(),
			}, {
				name:  "list",
				input: starlark.NewList([]starlark.Value{starlark.None}),
			}, {
				name:  "range",
				input: starlark.Range(0, 10, 1),
			}, {
				name:  "string",
				input: starlark.String("test"),
			}, {
				name:  "stringElems-chars",
				input: starlark.StringElems("test", false),
			}, {
				name:  "stringElems-ords",
				input: starlark.StringElems("test", true),
			}, {
				name:  "tuple",
				input: starlark.Tuple{starlark.None},
			}}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					st := startest.From(t)
					st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
					st.SetMinExecutionSteps(1)
					st.AddValue("input", test.input)
					st.RunString(`
						for _ in st.ntimes():
							st.keep_alive(input[0])
					`)
				})
			}
		})

		t.Run("set", func(t *testing.T) {
			tests := []struct {
				name  string
				input starlark.Value
			}{{
				name:  "dict",
				input: starlark.NewDict(1),
			}, {
				name:  "list",
				input: starlark.NewList([]starlark.Value{starlark.None}),
			}}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					st := startest.From(t)
					st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
					st.SetMinExecutionSteps(1)
					st.AddValue("input", test.input)
					st.RunString(`
						for _ in st.ntimes():
							input[0] = None
					`)
				})
			}
		})
	})
}

func TestFunctionCall(t *testing.T) {
	t.Run("vm-stack", func(t *testing.T) {
		stack_frame := starlark.NewBuiltinWithSafety(
			"stack_frame",
			starlark.MemSafe,
			func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
				frameSize := starlark.EstimateSize(starlark.StackFrameCapture{})
				if err := thread.AddAllocs(frameSize); err != nil {
					return nil, err
				}
				return thread.FrameAt(1), nil
			},
		)

		t.Run("shallow", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(stack_frame)
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				def empty():
					st.keep_alive(stack_frame())

				for _ in st.ntimes():
					empty()
			`)
		})

		t.Run("deep", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(stack_frame)
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				def recurse(depth=0):
					st.keep_alive(stack_frame())
					if depth < st.n:
						recurse(depth + 1)

				recurse()
			`)
		})

		t.Run("with-captures", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(stack_frame)
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				def run():
					sf = stack_frame()
					st.keep_alive(sf)
					def closure():
						st.keep_alive(sf, stack_frame())
					closure()

				for _ in st.ntimes():
					run()
			`)
		})
	})

	t.Run("builtin", func(t *testing.T) {
		keep_alive_args := func(st *startest.ST) *starlark.Builtin {
			return starlark.NewBuiltinWithSafety("keep_alive_args", starlark.MemSafe,
				func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
					if args != nil {
						argsSize := starlark.EstimateSize(starlark.Tuple{})
						if err := thread.AddAllocs(argsSize); err != nil {
							return nil, err
						}
						st.KeepAlive(args)
					}

					if kwargs != nil {
						kwargsSize := starlark.EstimateSize([]starlark.Tuple{})
						if err := thread.AddAllocs(kwargsSize); err != nil {
							return nil, err
						}
						st.KeepAlive(kwargs)
					}

					return starlark.None, nil
				})
		}

		t.Run("args", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(keep_alive_args(st))
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				for _ in st.ntimes():
					keep_alive_args(1, 1, 1, 1)
			`)
		})

		t.Run("varargs", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(keep_alive_args(st))
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				args = range(1, 10)
				for _ in st.ntimes():
					keep_alive_args(*args)
			`)
		})

		t.Run("mixed-args", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(keep_alive_args(st))
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				args = range(1, 10)
				for _ in st.ntimes():
					keep_alive_args(1, 2, 3, *args)
			`)
		})

		t.Run("kwargs", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(keep_alive_args(st))
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				for _ in st.ntimes():
					keep_alive_args(a=1, b=2, c=3)
			`)
		})

		t.Run("varkwargs", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(keep_alive_args(st))
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				kwargs = { "a": 1, "b": 2, "c": 3 }
				for _ in st.ntimes():
					keep_alive_args(**kwargs)
			`)
		})

		t.Run("mixed-kwargs", func(t *testing.T) {
			st := startest.From(t)
			st.AddBuiltin(keep_alive_args(st))
			st.RequireSafety(starlark.MemSafe)
			st.RunString(`
				kwargs = { "a": 1, "b": 2, "c": 3 }
				for _ in st.ntimes():
					keep_alive_args(d=4, e=5, **kwargs)
			`)
		})
	})
}
