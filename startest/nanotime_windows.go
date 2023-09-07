//+build windows

package startest

import (
	"syscall"
	"unsafe"
)

var (
	queryPerformanceCounter uintptr
	counterFrequency        int64
)

func init() {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	queryPerformanceFrequency := k32.NewProc("QueryPerformanceFrequency").Addr()
	syscall.Syscall(queryPerformanceFrequency, 1, uintptr(unsafe.Pointer(&counterFrequency)), 0, 0)
	queryPerformanceCounter = k32.NewProc("QueryPerformanceCounter").Addr()
}

func nanotime() int64 {
	var now int64
	syscall.Syscall(queryPerformanceCounter, 1, uintptr(unsafe.Pointer(&now)), 0, 0)
	return now * (1e9 / counterFrequency)
}
