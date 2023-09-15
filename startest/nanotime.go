//+build !windows

package startest

import (
	_ "unsafe"
	"time"
)

//go:linkname nanotime runtime.nanotime
func nanotime() time.Duration 
