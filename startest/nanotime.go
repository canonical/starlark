package startest

// instant represents a measurement of a monotonically nondecreasing clock, in
// nanoseconds. Its value is only meaningful to represent the start or end of a
// duration.
type instant int64

// nanotime returns the current high-precision instant. Unfortunately, the measures
// provided in the time package are not precise enough for very short durations,
// especially on Windows.
//
// See https://github.com/golang/go/issues/31160
//
//go:inline
func nanotime() instant { return instant(nanotime_impl()) }
