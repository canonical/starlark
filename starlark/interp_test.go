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
	stackFrame := starlark.NewBuiltinWithSafety("stack_frame", starlark.MemSafe, starlark.StackFrame)
	makeCaptureCall := func(st *startest.ST) *starlark.Builtin {
		return starlark.NewBuiltinWithSafety("capture_call", starlark.MemSafe,
			func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				if args != nil {
					if err := thread.AddAllocs(starlark.EstimateSize(starlark.Tuple{})); err != nil {
						return nil, err
					}
				}
				if kwargs != nil {
					if err := thread.AddAllocs(starlark.EstimateSize([]starlark.Tuple{})); err != nil {
						return nil, err
					}
				}
				st.KeepAlive(args, kwargs)
				return starlark.None, nil
			})
	}

	t.Run("stack", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(stackFrame)
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			def empty():
				st.keep_alive(stack_frame())
	
			for _ in st.ntimes():
				empty()
		`)
	})

	t.Run("frame", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(stackFrame)
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			def recurse(i):
				st.keep_alive(stack_frame())
				if i:
					recurse(i-1)
	
			recurse(st.n)
		`)
	})

	t.Run("closure", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(stackFrame)
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

	t.Run("builtin-positional", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(makeCaptureCall(st))
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			for _ in st.ntimes():
				capture_call(1, 1, 1, 1)
		`)
	})

	t.Run("builtin-varargs", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(makeCaptureCall(st))
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			for _ in st.ntimes():
				capture_call(*range(1, 10))
		`)
	})

	t.Run("builtin-mixed-args", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(makeCaptureCall(st))
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			for _ in st.ntimes():
				capture_call(1, 2, 3, *range(1, 10))
		`)
	})

	t.Run("builtin-named", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(makeCaptureCall(st))
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			for _ in st.ntimes():
				capture_call(a=1, b=2, c=3)
		`)
	})

	t.Run("builtin-kwargs", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(makeCaptureCall(st))
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			for _ in st.ntimes():
				args = { "a": 1, "b": 2, "c": 3 }
				capture_call(**args)
		`)
	})

	t.Run("builtin-mixed-kwargs", func(t *testing.T) {
		st := startest.From(t)
		st.AddBuiltin(makeCaptureCall(st))
		st.RequireSafety(starlark.MemSafe)
		st.RunString(`
			for _ in st.ntimes():
				args = { "a": 1, "b": 2, "c": 3 }
				capture_call(d=4, e=5, **args)
		`)
	})
}
