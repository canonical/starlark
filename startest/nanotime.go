//+build !windows

package startest

import _ "unsafe"

//go:linkname nanotime runtime.nanotime
func nanotime() int64
