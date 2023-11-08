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
	inputs := []starlark.HasAttrs{
		starlark.NewList(nil),
		starlark.NewDict(1),
		starlark.NewSet(1),
		starlark.String("1"),
		starlark.Bytes("1"),
	}
	for _, input := range inputs {
		attr := input.AttrNames()[0]
		t.Run(input.Type(), func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.AddValue("input", input)
			st.RunString(fmt.Sprintf(`
				for _ in st.ntimes():
					st.keep_alive(input.%s)
			`, attr))
		})
	}
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
