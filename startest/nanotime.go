//+build !windows

package startest

import (
	_ "unsafe" // for go:linkname hack
)

type instant int64

//go:linkname runtime_nanotime runtime.nanotime
func runtime_nanotime() int64

//go:inline
func nanotime() instant {
	return instant(runtime_nanotime())
}
