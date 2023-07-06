package starlark_test

import (
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

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

func TestUnary(t *testing.T) {
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
