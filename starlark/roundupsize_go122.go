//go:build go1.22
// +build go1.22

package starlark

import _ "unsafe"

//go:linkname runtime_roundupsize runtime.roundupsize
func runtime_roundupsize(size uintptr, noscan bool) (reqSize uintptr)

func roundupsize(size uintptr) uintptr {
	// Go 1.22 adds 8 bytes of padding for 'scan' values (those containing
	// pointers). For simplicity, we always assume this overhead is present.
	return runtime_roundupsize(size+8, true)
}
