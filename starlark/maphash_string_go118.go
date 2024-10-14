//go:build !go1.19
// +build !go1.19

package starlark

import _ "unsafe"

//go:linkname runtime_stringhash runtime.stringHash
func runtime_stringhash(s string, seed uintptr) uintptr

func maphash_string(s string) uint32 {
	return uint32(runtime_stringhash(s, 0))
}
