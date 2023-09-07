package startest

import (
	"time"

	"github.com/canonical/starlark/starlark"
)

const STSafe = stSafe

func STSafety(st *ST) starlark.Safety {
	return st.requiredSafety
}

func Nanotime() time.Duration { return time.Duration(nanotime()) }
