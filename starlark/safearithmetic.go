package starlark

import (
	"math"
	"math/bits"
)

func SafeAdd(a, b int) int {
	if a == math.MinInt || a == math.MaxInt {
		return a
	}
	if b == math.MinInt || b == math.MaxInt {
		return b
	}

	if ret := int(uint(a) + uint(b)); !sameSign(a, b) || sameSign(ret, a) {
		// no overflow possible
		return ret
	}

	if a >= 0 {
		return math.MaxInt
	}
	return math.MinInt
}

//go:inline
func sameSign(a, b int) bool {
	return a^b >= 0
}

func SafeAdd64(a, b int64) int64 {
	if a == math.MinInt64 || a == math.MaxInt64 {
		return a
	}
	if b == math.MinInt64 || b == math.MaxInt64 {
		return b
	}

	if ret := int64(uint64(a) + uint64(b)); !sameSign64(a, b) || sameSign64(ret, a) {
		// no overflow possible
		return ret
	}

	if a >= 0 {
		return math.MaxInt64
	}
	return math.MinInt64
}

//go:inline
func sameSign64(a, b int64) bool {
	return a^b >= 0
}

func SafeMul(a, b int) int {
	if a == math.MinInt || a == math.MaxInt {
		return a
	}
	if b == math.MinInt || b == math.MaxInt {
		return b
	}

	if a == 0 {
		return 0
	}
	if ret := a * b; ret/a == b {
		// No overflow occurred.
		return ret
	}

	if (a >= 0) != (b >= 0) {
		return math.MinInt
	}
	return math.MaxInt
}

func SafeMul64(a, b int64) int64 {
	if a == math.MinInt64 || a == math.MaxInt64 {
		return a
	}
	if b == math.MinInt64 || b == math.MaxInt64 {
		return b
	}

	if a == 0 {
		return 0
	}
	if ret := a * b; ret/a == b {
		// No overflow occurred.
		return ret
	}

	if (a >= 0) != (b >= 0) {
		return math.MinInt64
	}
	return math.MaxInt64
}
