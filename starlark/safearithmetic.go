package starlark

import (
	"math"
	"math/bits"
)

func SafeAdd(a, b int) int {
	switch a {
	case math.MinInt, math.MaxInt:
		return a
	}
	switch b {
	case math.MinInt, math.MaxInt:
		return b
	}

	if a < 0 {
		negatedRet := SafeAdd(-a, -b)
		switch negatedRet {
		case math.MaxInt:
			return math.MinInt
		case math.MinInt:
			return math.MaxInt
		}
		return -negatedRet
	}

	if b >= 0 {
		sum, carry := bits.Add(uint(a), uint(b), 0)
		if sum > math.MaxInt || carry != 0 {
			return math.MaxInt
		}
		return int(sum)
	}

	if uint(a) < uint(-b) {
		return math.MinInt
	}
	diff, _ := bits.Sub(uint(a), uint(-b), 0)
	return int(diff)
}

func SafeAdd64(a, b int64) int64 {
	switch a {
	case math.MinInt64, math.MaxInt64:
		return a
	}
	switch b {
	case math.MinInt64, math.MaxInt64:
		return b
	}

	if a < 0 {
		negatedRet := SafeAdd64(-a, -b)
		switch negatedRet {
		case math.MaxInt64:
			return math.MinInt64
		case math.MinInt64:
			return math.MaxInt64
		}
		return -negatedRet
	}

	if b >= 0 {
		sum, carry := bits.Add64(uint64(a), uint64(b), 0)
		if sum > math.MaxInt64 || carry != 0 {
			return math.MaxInt64
		}
		return int64(sum)
	}

	if uint64(a) < uint64(b) {
		return math.MinInt64
	}
	diff, _ := bits.Sub64(uint64(a), uint64(-b), 0)
	return int64(diff)
}

func SafeMul(a, b int) int {
	var retSign int
	if (a > 0) == (b > 0) {
		retSign = 1
	} else {
		retSign = -1
	}

	if a == math.MinInt || a == math.MaxInt || b == math.MinInt || b == math.MaxInt {
		if retSign > 0 {
			return math.MaxInt
		} else {
			return math.MinInt
		}
	}

	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	hi, lo := bits.Mul(uint(a), uint(b))
	if hi == 0 && lo < math.MaxInt {
		return retSign * int(lo)
	}
	if retSign > 0 {
		return math.MaxInt
	}
	return math.MinInt
}

func SafeMul64(a, b int64) int64 {
	var retSign int64
	if (a > 0) == (b > 0) {
		retSign = 1
	} else {
		retSign = -1
	}

	if a == math.MinInt64 || a == math.MaxInt64 || b == math.MinInt64 || b == math.MaxInt64 {
		if retSign > 0 {
			return math.MaxInt64
		} else {
			return math.MinInt64
		}
	}

	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if hi == 0 && lo < math.MaxInt64 {
		return retSign * int64(lo)
	}
	if retSign > 0 {
		return math.MaxInt64
	}
	return math.MinInt64
}
