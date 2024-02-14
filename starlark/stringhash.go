//go:build !tinygo
// +build !tinygo

package starlark

import _ "unsafe"

//go:linkname goStringHash runtime.stringHash
func goStringHash(s string, seed uintptr) uintptr
