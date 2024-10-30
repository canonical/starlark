package starlark

import (
	"math"
)

func SafeAddInt(a, b int) int {
	if a == math.MinInt || a == math.MaxInt {
		return a
	}
	if b == math.MinInt || b == math.MaxInt {
		return b
	}

	if ret := int(uint(a) + uint(b)); !sameSignInt(a, b) || sameSignInt(ret, a) {
		// no overflow possible
		return ret
	}

	if a >= 0 {
		return math.MaxInt
	}
	return math.MinInt
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

func SafeMulInt(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}

	if a != math.MinInt && a != math.MaxInt && b != math.MinInt && b != math.MaxInt {
		if ab := a * b; ab/a == b {
			// No overflow occurred.
			return ab
		}
	}

	if sameSignInt(a, b) {
		return math.MaxInt
	}
	return math.MinInt
}

func SafeMul64(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}

	if a != math.MinInt64 && a != math.MaxInt64 && b != math.MinInt64 && b != math.MaxInt64 {
		if ab := a * b; ab/a == b {
			// No overflow occurred.
			return ab
		}
	}

	if sameSign64(a, b) {
		return math.MaxInt64
	}
	return math.MinInt64
}

//go:inline
func sameSignInt(a, b int) bool {
	return a^b >= 0
}

//go:inline
func sameSign64(a, b int64) bool {
	return a^b >= 0
}
