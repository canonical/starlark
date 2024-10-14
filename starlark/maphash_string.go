//go:build go1.19
// +build go1.19

package starlark

import "hash/maphash"

var seed = maphash.MakeSeed()

func maphash_string(s string) uint32 {
	h := maphash.String(seed, s)
	return uint32(h>>32) | uint32(h)
}
