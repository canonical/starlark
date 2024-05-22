//go:build go1.22
// +build go1.22

package starlark

import _ "unsafe"

//go:linkname runtime_roundupsize runtime.roundupsize
func runtime_roundupsize(size uintptr, noscan bool) (reqSize uintptr)

func roundupsize(size uintptr) uintptr {
	// In go1.22+, "scan" values (values that contain pointers) require
	// an additional pointer to the type (8 bytes). It is ok to overestimate
	// and always add this overhead to avoid checking if a type is noscan.
	return runtime_roundupsize(size, false)
}
