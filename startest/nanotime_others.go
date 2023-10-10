//go:build !windows
// +build !windows

package startest

import (
	_ "unsafe" // for go:linkname hack
)

//go:linkname nanotime_impl runtime.nanotime
func nanotime_impl() int64
