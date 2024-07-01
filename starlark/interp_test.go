package starlark_test

import (
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestUnary(t *testing.T) {
	t.Run("not", func(t *testing.T) {
		tests := []struct {
			name  string
			input starlark.Value
		}{{
			name:  "None",
			input: starlark.None,
		}, {
			name:  "True",
			input: starlark.True,
		}, {
			name:  "Tuple",
			input: starlark.Tuple{},
		}, {
			name:  "Int",
			input: starlark.MakeInt(1),
		}, {
			name:  "Float",
			input: starlark.Float(1),
		}, {
			name:  "List",
			input: starlark.NewList(nil),
		}, {
			name:  "Dict",
			input: starlark.NewDict(1),
		}, {
			name:  "Set",
			input: starlark.NewSet(1),
		}, {
			name:  "String",
			input: starlark.String("1"),
		}, {
			name:  "Bytes",
			input: starlark.Bytes("1"),
		}}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
				st.SetMinSteps(1)
				st.AddValue("input", test.input)
				st.RunString(`
					for _ in st.ntimes():
						st.keep_alive(not input)
				`)
			})
		}
	})

	t.Run("minus", func(t *testing.T) {
		tests := []struct {
			name  string
			input starlark.Value
		}{{
			name:  "Int (small)",
			input: starlark.MakeInt(10),
		}, {
			name:  "Int (big)",
			input: starlark.MakeInt64(1 << 40),
		}, {
			name:  "Float",
			input: starlark.Float(1),
		}}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
				st.SetMinSteps(1)
				st.AddValue("input", test.input)
				st.RunString(`
					for _ in st.ntimes():
						st.keep_alive(-input)
				`)
			})
		}
	})

	t.Run("plus", func(t *testing.T) {
		tests := []struct {
			name  string
			input starlark.Value
		}{{
			name:  "Int (small)",
			input: starlark.MakeInt(10),
		}, {
			name:  "Int (big)",
			input: starlark.MakeInt64(1 << 40),
		}, {
			name:  "Float",
			input: starlark.Float(1),
		}}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
				st.SetMinSteps(1)
				st.AddValue("input", test.input)
				st.RunString(`
					for _ in st.ntimes():
						st.keep_alive(+input)
				`)
			})
		}
	})

	t.Run("neg", func(t *testing.T) {
		tests := []struct {
			name  string
			input starlark.Value
		}{{
			name:  "Int (small)",
			input: starlark.MakeInt(10),
		}, {
			name:  "Int (big)",
			input: starlark.MakeInt64(1 << 40),
		}}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
				st.SetMinSteps(1)
				st.AddValue("input", test.input)
				st.RunString(`
					for _ in st.ntimes():
						st.keep_alive(~input)
				`)
			})
		}
	})
}

func TestTupleCreation(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(1)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive(())
		`)
	})

	t.Run("not-empty", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(4)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive((1, "2", 3.0))
		`)
	})
}

func TestListCreation(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(1)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive([])
		`)
	})

	t.Run("not-empty", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(4)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive([ False, 1, "2", 3.0 ])
		`)
	})
}

func TestListComprehension(t *testing.T) {
	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		// The step cost per N is at least 7:
		// - For creating the list, 1
		// - For loading the constant, 1
		// - For calling range, 1
		// - For iterating twice, 2
		// - For appending twice, 2
		st.SetMinSteps(7)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive([v for v in range(2)])
		`)
	})

	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(2)
		st.RunString(`
			st.keep_alive([v for v in range(st.n)])
		`)
	})
}

func TestDictCreation(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(1)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive({})
		`)
	})

	t.Run("not-empty", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		// The step cost per N is at least 10:
		// - For creating the dict, 1
		// - For loading the keys, 3
		// - For loading the values, 3
		// - For adding the items, 3
		st.SetMinSteps(10)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive({ 1: False, 2: "2", 3: 3.0 })
		`)
	})
}

func TestDictComprehension(t *testing.T) {
	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		// The step cost per N is at least 9:
		// - For creating the dict, 1
		// - For loading the constant, 1
		// - For calling range, 1
		// - For iterating twice, 2
		// - For loading the constant twice, 2
		// - For appending twice, 2
		st.SetMinSteps(9)
		st.RunString(`
			for _ in st.ntimes():
				st.keep_alive({i: None for i in range(2)})
		`)
	})

	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		// The step cost per N is at least:
		// - For iterating, 1
		// - For loading the constant, 1
		// - For appending, 1
		st.SetMinSteps(3)
		st.RunString(`
			st.keep_alive({i: None for i in range(st.n)})
		`)
	})
}

func TestIterate(t *testing.T) {
	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(2)
		st.RunString(`
			for _ in st.ntimes():
				for j in range(2):
					st.keep_alive(j)
					break
		`)
	})

	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(1)
		st.RunString(`
			for _ in range(st.n):
				pass
		`)
	})
}

func TestSequenceAssignment(t *testing.T) {
	t.Run("list-single", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(2)
		st.RunString(`
			for _ in st.ntimes():
				[ single ] = range(1)
				st.keep_alive(single)
		`)
	})

	t.Run("list-double", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(4)
		st.RunString(`
			for _ in st.ntimes():
				[ first, second ] = range(2)
				st.keep_alive(first, second)
		`)
	})

	t.Run("double", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(4)
		st.RunString(`
			for _ in st.ntimes():
				first, second = range(2)
				st.keep_alive(first, second)
		`)
	})
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
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMinSteps(1)
			st.AddValue("input", test.input)
			st.RunString(fmt.Sprintf(`
				for _ in st.ntimes():
					st.keep_alive(input.%s)
			`, test.attr))
		})
	}
}

type unsafeTestSetField struct{}

var _ starlark.HasSetField = &unsafeTestSetField{}

func (utsf *unsafeTestSetField) Freeze()              {}
func (utsf *unsafeTestSetField) String() string       { return "unsafeTestSetField" }
func (utsf *unsafeTestSetField) Truth() starlark.Bool { return starlark.False }
func (utsf *unsafeTestSetField) Type() string         { return "<unsafeTestSetField>" }
func (utsf *unsafeTestSetField) AttrNames() []string  { return nil }

func (utsf *unsafeTestSetField) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", utsf.Type())
}

func (utsf *unsafeTestSetField) Attr(name string) (starlark.Value, error) {
	return nil, starlark.ErrNoAttr
}

func (utsf *unsafeTestSetField) SetField(name string, val starlark.Value) error {
	return fmt.Errorf("SetField called")
}

type testSetField struct {
	safety        starlark.SafetyFlags
	steps, allocs int64
}

var _ starlark.HasSafeSetField = &testSetField{}

func (tsf *testSetField) Freeze()              {}
func (tsf *testSetField) String() string       { return "testSetField" }
func (tsf *testSetField) Truth() starlark.Bool { return starlark.False }
func (tsf *testSetField) Type() string         { return "<testSetField>" }
func (tsf *testSetField) AttrNames() []string  { return nil }

func (tsf *testSetField) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", tsf.Type())
}

func (tsf *testSetField) Attr(name string) (starlark.Value, error) {
	return nil, starlark.ErrNoAttr

}
func (tsf *testSetField) SetField(name string, val starlark.Value) error {
	return tsf.SafeSetField(nil, name, val)
}

func (tsf *testSetField) SafeSetField(thread *starlark.Thread, name string, val starlark.Value) error {
	if err := starlark.CheckSafety(thread, tsf.safety); err != nil {
		return err
	}
	if thread != nil {
		if err := thread.AddAllocs(tsf.allocs); err != nil {
			return err
		}
		if err := thread.AddSteps(tsf.steps); err != nil {
			return err
		}
	}
	return nil
}

func TestSetField(t *testing.T) {
	t.Run("safety-respected", func(t *testing.T) {
		dummy := &testing.T{}
		st := startest.From(dummy)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.AddValue("input", &unsafeTestSetField{})
		ok := st.RunString(`
			input.field = 1
		`)
		if ok {
			st.Error("expected error")
		}
	})

	t.Run("not-safe", func(t *testing.T) {
		dummy := &testing.T{}
		st := startest.From(dummy)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.AddValue("input", &testSetField{})
		ok := st.RunString(`
			for _ in st.ntimes():
				input.field = 1
		`)
		if ok {
			st.Error("expected error")
		}
	})

	t.Run("safe", func(t *testing.T) {
		const safety = starlark.CPUSafe | starlark.MemSafe

		st := startest.From(t)
		st.RequireSafety(safety)
		st.AddValue("input", &testSetField{
			safety: safety,
			steps:  100,
			allocs: 100,
		})
		st.SetMaxAllocs(100)
		st.SetMinSteps(100)
		st.RunString(`
			for _ in st.ntimes():
				input.field = 1
		`)
	})
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
					st.RequireSafety(starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe)
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
					st.RequireSafety(starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe)
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
					st.RequireSafety(starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe)
					st.SetMinSteps(1)
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
					st.RequireSafety(starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe)
					st.SetMinSteps(1)
					st.AddValue("input", test.input)
					st.RunString(`
						for _ in st.ntimes():
							input[0] = None
					`)
				})
			}
		})

		t.Run("cancellation", func(t *testing.T) {
			t.Run("dict", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.TimeSafe)
				st.SetMaxSteps(0)
				st.RunThread(func(thread *starlark.Thread) {
					thread.Cancel("done")
					dict := starlark.NewDict(st.N)
					for i := 0; i < st.N; i++ {
						// Int hash only uses the least 32 bits.
						// Leaving them blank creates collisions.
						key := starlark.MakeInt64(int64(i) << 32)
						dict.SetKey(key, starlark.None)
					}
					_, _, err := dict.SafeGet(thread, starlark.None)
					if err == nil {
						st.Error("expected cancellation")
					} else if !isStarlarkCancellation(err) {
						st.Errorf("expected cancellation, got: %v", err)
					}
					err = dict.SafeSetKey(thread, starlark.MakeInt64(int64(st.N)<<32), starlark.None)
					if err == nil {
						st.Error("expected cancellation")
					} else if !isStarlarkCancellation(err) {
						st.Errorf("expected cancellation, got: %v", err)
					}
				})
			})
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
