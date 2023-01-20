package startest

import "github.com/canonical/starlark/starlark"

const STSafe = stSafe

func STSafety(st *ST) starlark.Safety {
	return st.requiredSafety
}
