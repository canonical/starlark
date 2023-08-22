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

func TestDictCreation(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunString(`
		for _ in st.ntimes():
			st.keep_alive({})
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
