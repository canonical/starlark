//go:build tinygo && gc.precise

package starlark

import "unsafe"

const (
	wordSize      = unsafe.Sizeof(unsafe.Pointer(nil))
	wordsPerBlock = 4
	bytesPerBlock = wordsPerBlock * wordSize
)

func roundupsize(size uintptr) uintptr {
	blocks := (size + wordSize + (bytesPerBlock - 1)) / bytesPerBlock
	return blocks * bytesPerBlock
}
