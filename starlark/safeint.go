package starlark

import (
	"math"
)

// Marker value to indicate that an overflow has occurred,
// NB: As this value is equal to MinInt64, the space of valid
// safe integers is closed under negation.
const invalidSafeInt = math.MinInt64

type Signed interface {
	int | int8 | int16 | int32 | int64
}

type Unsigned interface {
	uint | uint8 | uint16 | uint32 | uint64
}

type Integer interface {
	Signed | Unsigned
}

// SafeInteger represent an overflow-safe integer.
type SafeInteger struct {
	value int64
}

func SafeInt[Int Integer | SafeInteger](i Int) SafeInteger {
	switch i := any(i).(type) {
	case SafeInteger:
		return i
	case int:
		return SafeInteger{int64(i)}
	case int8:
		return SafeInteger{int64(i)}
	case int16:
		return SafeInteger{int64(i)}
	case int32:
		return SafeInteger{int64(i)}
	case int64:
		return SafeInteger{i}
	case uint8:
		return SafeInteger{int64(i)}
	case uint16:
		return SafeInteger{int64(i)}
	case uint32:
		return SafeInteger{int64(i)}
	case uint:
		if uint64(i) > math.MaxInt64 {
			return SafeInteger{invalidSafeInt}
		}
		return SafeInteger{int64(i)}
	case uint64:
		if i > math.MaxInt64 {
			return SafeInteger{invalidSafeInt}
		}
		return SafeInteger{int64(i)}
	default:
		panic("unreachable")
	}
}

func (si SafeInteger) Int64() (int64, bool) {
	if si.value == invalidSafeInt {
		return 0, false
	}
	return si.value, true
}

func (si SafeInteger) Uint64() (uint64, bool) {
	i64, ok := si.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 {
		return 0, false
	}
	return uint64(i64), true
}

func (si SafeInteger) Int() (int, bool) {
	i64, ok := si.Int64()
	if !ok {
		return 0, false
	}
	if i64 < math.MinInt || math.MaxInt < i64 {
		return 0, false
	}
	return int(i64), true
}

func (si SafeInteger) Uint() (uint, bool) {
	i64, ok := si.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || math.MaxUint < uint(i64) {
		return 0, false
	}
	return uint(i64), true
}

func (si SafeInteger) Int32() (int32, bool) {
	i64, ok := si.Int64()
	if !ok {
		return 0, false
	}
	if i64 < math.MinInt32 || math.MaxInt32 < i64 {
		return 0, false
	}
	return int32(i64), true
}

func (si SafeInteger) Uint32() (uint32, bool) {
	i64, ok := si.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || math.MaxUint32 < i64 {
		return 0, false
	}
	return uint32(i64), true
}

func (si SafeInteger) Int16() (int16, bool) {
	i64, ok := si.Int64()
	if !ok {
		return 0, false
	}
	if i64 < math.MinInt16 || math.MaxInt16 < i64 {
		return 0, false
	}
	return int16(i64), true
}

func (i SafeInteger) Uint16() (uint16, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || i64 > math.MaxUint16 {
		return 0, false
	}
	return uint16(i64), true
}

func (i SafeInteger) Int8() (int8, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < math.MinInt8 || math.MaxInt8 < i64 {
		return 0, false
	}
	return int8(i64), true
}

func (i SafeInteger) Uint8() (uint8, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || math.MaxUint8 < i64 {
		return 0, false
	}
	return uint8(i64), true
}

func SafeNeg[Int Integer | SafeInteger](i Int) SafeInteger {
	si := SafeInt(i)
	// Note: math.MinInt64 == -math.MinInt64, so
	// no need to check as the invalid value
	// remains stable on sign change.
	return SafeInteger{-si.value}
}

func SafeAdd[A, B Integer | SafeInteger](a A, b B) SafeInteger {
	sa, sb := SafeInt(a), SafeInt(b)
	if invalid(sa, sb) {
		return SafeInteger{invalidSafeInt}
	}
	return SafeInteger{safeAdd(sa.value, sb.value)}
}

func safeAdd(a, b int64) int64 {
	if ret := a + b; !sameSign64(a, b) || sameSign64(ret, a) {
		// no overflow possible
		return ret
	}
	return invalidSafeInt
}

func SafeSub[A, B Integer | SafeInteger](a A, b B) SafeInteger {
	return SafeAdd(a, SafeNeg(b))
}

func SafeMul[A, B Integer | SafeInteger](a A, b B) SafeInteger {
	sa, sb := SafeInt(a), SafeInt(b)
	if invalid(sa, sb) {
		return SafeInteger{invalidSafeInt}
	}
	return SafeInteger{safeMul(sa.value, sb.value)}
}

func safeMul(a, b int64) int64 {
	if ab := a * b; ab/a == b {
		// No overflow occurred.
		return ab
	}
	return invalidSafeInt
}

func SafeDiv[A, B Integer | SafeInteger](a A, b B) SafeInteger {
	sa, sb := SafeInt(a), SafeInt(b)
	if invalid(sa, sb) {
		return SafeInteger{invalidSafeInt}
	}
	return SafeInteger{safeDiv(sa.value, sb.value)}
}

func safeDiv(a, b int64) int64 {
	if b == 0 {
		return invalidSafeInt
	}
	return a / b
}

func invalid(ints ...SafeInteger) bool {
	for _, i := range ints {
		if i.value == invalidSafeInt {
			return true
		}
	}
	return false
}
