package time

import "github.com/canonical/starlark/starlark"

var Safeties = safeties
var TimeMethods = timeMethods
var TimeMethodSafeties = timeMethodSafeties

func SafeNowFunc() func(thread *starlark.Thread) (Time, error) {
	return safeNowFunc
}

func SafeNowFuncSafety() starlark.Safety {
	return safeNowFuncSafety
}
