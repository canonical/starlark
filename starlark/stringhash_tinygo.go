//go:build tinygo
// +build tinygo

package starlark

import _ "unsafe"

//go:linkname goStringHash runtime.hashmapStringHash
func goStringHash(s string, seed uintptr) uintptr
