//go:build go1.22
// +build go1.22

package starlark

import _ "unsafe"

//go:linkname runtime_roundupsize runtime.roundupsize
func runtime_roundupsize(size uintptr, noscan bool) (reqSize uintptr)

func roundupsize(size uintptr) uintptr {
	return runtime_roundupsize(size, false)
}
