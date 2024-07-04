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

	if b >= 0 {
		sum, carry := bits.Add(uint(a), uint(b), 0)
		if int(sum) < 0 || carry != 0 {
			return math.MaxInt
		}
		return int(sum)
	}

	diff, carry := bits.Sub(uint(a), uint(-b), 0)
	if int(diff) < 0 || carry != 0 {
		return math.MinInt
	}
	return int(diff)
}

func SafeAdd64(a, b int64) int64 {
	if a == math.MinInt64 || a == math.MaxInt64 {
		return a
	}
	if b == math.MinInt64 || b == math.MaxInt64 {
		return b
	}

	if b >= 0 {
		sum, carry := bits.Add64(uint64(a), uint64(b), 0)
		if int64(sum) < 0 || carry != 0 {
			return math.MaxInt64
		}
		return int64(sum)
	}

	diff, carry := bits.Sub64(uint64(a), uint64(-b), 0)
	if int(diff) < 0 || carry != 0 {
		return math.MinInt64
	}
	return int64(diff)
}

func SafeMul(a, b int) int {
	hi, lo := bits.Mul(uint(a), uint(b))

	expectPositive := (a > 0) == (b > 0)
	if expectPositive != (lo > 0) || hi != 0 {
		if expectPositive {
			return math.MinInt
		}
		return math.MaxInt
	}

	return int(lo)
}

func SafeMul64(a, b int64) int64 {
	hi, lo := bits.Mul64(uint64(a), uint64(b))

	expectPositive := (a > 0) == (b > 0)
	if expectPositive != (lo > 0) || hi != 0 {
		if expectPositive {
			return math.MinInt
		}
		return math.MaxInt
	}

	return int64(lo)
}
