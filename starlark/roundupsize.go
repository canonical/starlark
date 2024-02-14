//go:build !tinygo

package starlark

import _ "unsafe"

//go:linkname roundupsize runtime.roundupsize
func roundupsize(size uintptr) uintptr
