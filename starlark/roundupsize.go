//go:build !go1.22
// +build !go1.22

package starlark

import _ "unsafe"

//go:linkname roundupsize runtime.roundupsize
func roundupsize(size uintptr) uintptr
