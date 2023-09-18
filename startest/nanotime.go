//+build !windows

package startest

import (
	_ "unsafe"
	"time"
)

//go:linkname runtime_nanotime runtime.nanotime
func runtime_nanotime() int64

//go:inline
func nanotime() time.Duration {
	return time.Duration(runtime_nanotime())
}
