//go:build windows
// +build windows

package startest

import (
	"syscall"
	"unsafe"
)

var (
	queryPerformanceCounter          uintptr
	queryPerformanceCounterFrequency int64
)

func init() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	queryPerformanceFrequency := kernel32.NewProc("QueryPerformanceFrequency").Addr()
	syscall.Syscall(queryPerformanceFrequency, 1, uintptr(unsafe.Pointer(&queryPerformanceCounterFrequency)), 0, 0)
	queryPerformanceCounter = kernel32.NewProc("QueryPerformanceCounter").Addr()
}

func nanotime() int64 {
	var now int64
	syscall.Syscall(queryPerformanceCounter, 1, uintptr(unsafe.Pointer(&now)), 0, 0)
	return now * (1e9 / queryPerformanceCounterFrequency)
}
