package startest

import "github.com/canonical/starlark/starlark"

const StSafe = stSafe

func STSafety(st *ST) starlark.Safety {
	return st.requiredSafety
}
