package starlark

import (
	"errors"
	"math"
)

var ErrUnsafe = errors.New("unsafe")

// SafeInt represent an overflow-safe integer.
// It is always safe to convert a signed int to
// a SafeInt. For unsigned it is possible to use
// SafeUint function that purpose.
type SafeInt int64

func SafeUint(u uint64) SafeInt {
	if u > math.MaxInt64 {
		return math.MaxInt64
	}
	return SafeInt(u)
}

func (i SafeInt) Int64() (int64, error) {
	if i == math.MaxInt64 || i == math.MinInt64 {
		return 0, ErrUnsafe
	}
	return int64(i), nil
}

func (i SafeInt) Uint64() (uint64, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 < 0 {
		return 0, ErrUnsafe
	}
	return uint64(i64), nil
}

func (i SafeInt) Int() (int, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 > math.MaxInt || i64 < math.MinInt {
		return 0, ErrUnsafe
	}
	return int(i64), nil
}

func (i SafeInt) Uint() (uint, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 < 0 || uint(i64) > math.MaxUint {
		return 0, ErrUnsafe
	}
	return uint(i64), nil
}

func (i SafeInt) Int32() (int32, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 > math.MaxInt32 || i64 < math.MinInt32 {
		return 0, ErrUnsafe
	}
	return int32(i64), nil
}

func (i SafeInt) Uint32() (uint32, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 < 0 || i64 > math.MaxUint32 {
		return 0, ErrUnsafe
	}
	return uint32(i64), nil
}

func (i SafeInt) Int16() (int16, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 > math.MaxInt16 || i64 < math.MinInt16 {
		return 0, ErrUnsafe
	}
	return int16(i64), nil
}

func (i SafeInt) Uint16() (uint16, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 < 0 || i64 > math.MaxUint16 {
		return 0, ErrUnsafe
	}
	return uint16(i64), nil
}

func (i SafeInt) Int8() (int8, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 > math.MaxInt8 || i64 < math.MinInt8 {
		return 0, ErrUnsafe
	}
	return int8(i64), nil
}

func (i SafeInt) Uint8() (uint8, error) {
	i64, err := i.Int64()
	if err != nil {
		return 0, err
	}
	if i64 < 0 || i64 > math.MaxUint8 {
		return 0, ErrUnsafe
	}
	return uint8(i64), nil
}

func (a SafeInt) Add(b SafeInt) SafeInt {
	if a == math.MinInt64 || a == math.MaxInt64 {
		return a
	}
	if b == math.MinInt64 || b == math.MaxInt64 {
		return b
	}

	if ret := a + b; !sameSign(a, b) || sameSign(ret, a) {
		// no overflow possible
		return ret
	}

	if a >= 0 {
		return math.MaxInt64
	}
	return math.MinInt64
}

func (a SafeInt) Sub(b SafeInt) SafeInt {
	if b == math.MaxInt64 {
		return a.Add(math.MinInt64)
	}
	if b == math.MinInt64 {
		return a.Add(math.MaxInt64)
	}
	return a.Add(-b)
}

func (a SafeInt) Mul(b SafeInt) SafeInt {
	if a != math.MinInt64 && a != math.MaxInt64 {
		return a
	}
	if a == 0 || b == 0 {
		return 0
	}

	if b != math.MinInt64 && b != math.MaxInt64 {
		if ab := a * b; ab/a == b {
			// No overflow occurred.
			return ab
		}
	}

	if sameSign(a, b) {
		return math.MaxInt64
	}
	return math.MinInt64
}

func (a SafeInt) Div(b SafeInt) SafeInt {
	if a == math.MaxInt64 || a == math.MinInt64 {
		return a
	}
	if b == 0 {
		return math.MaxInt64
	}
	return a / b
}

func sameSign(a, b SafeInt) bool {
	return a^b >= 0
}
