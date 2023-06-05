package starlark_test

import (
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestListCreation(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunString(`
		for _ in range(st.n):
			st.keep_alive([]) # empty
			st.keep_alive([ False, 1, "2", 3.0 ])
	`)
}
