package starlark_test

import (
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestUnaryAllocs(t *testing.T) {
	t.Run("not", func(t *testing.T) {
		values := []starlark.Value{
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
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		for _, value := range values {
			st.AddValue("value", value)
			st.RunString(`
				for _ in st.ntimes():
					st.keep_alive(not value)
			`)
		}
	})

	t.Run("minus", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		for _, value := range []starlark.Value{starlark.MakeInt(10), starlark.MakeInt64(1 << 40)} {
			st.AddValue("value", value)
			st.RunString(`
				i = value
				for _ in st.ntimes():
					i = -i
					st.keep_alive(i)
			`)
		}
	})

	t.Run("plus", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		for _, value := range []starlark.Value{starlark.MakeInt(10), starlark.MakeInt64(1 << 40)} {
			st.AddValue("value", value)
			st.RunString(`
				i = value
				for _ in st.ntimes():
					i = +i
					st.keep_alive(i)
			`)
		}
	})

	t.Run("neg", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		for _, value := range []starlark.Value{starlark.MakeInt(10), starlark.MakeInt64(1 << 40)} {
			st.AddValue("value", value)
			st.RunString(`
				i = value
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
