package startest

import "github.com/canonical/starlark/starlark"

func STSafety(st *ST) starlark.Safety {
	return st.requiredSafety
}
