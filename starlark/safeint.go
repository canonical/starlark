package starlark

import (
	"fmt"
	"math"
)

// SafeInteger represents an overflow-safe integer.
type SafeInteger struct {
	value int64
}

var _ fmt.Stringer = SafeInteger{}

func (si SafeInteger) String() string {
	if !si.Valid() {
		return "SafeInt(invalid)"
	}
	return fmt.Sprintf("SafeInt(%d)", si.value)
}

// Marker value to indicate that an overflow has occurred,
// NB: As this value is equal to MinInt64, the space of valid
// safe integers is closed under negation.
const invalidSafeInt = math.MinInt64

var InvalidSafeInt = SafeInteger{math.MinInt64}

type Integer interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 | uintptr
}

type Floating interface {
	float32 | float64
}

var (
	minValidFloat32 = math.Nextafter32(math.MinInt64, 0)
	maxValidFloat32 = math.Nextafter32(math.MaxInt64, 0)
	minValidFloat64 = math.Nextafter(math.MinInt64, 0)
	maxValidFloat64 = math.Nextafter(math.MaxInt64, 0)
)

func SafeInt[I Integer | SafeInteger | Floating](i I) SafeInteger {
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
	case uint:
		if uint64(i) > math.MaxInt64 {
			return SafeInteger{invalidSafeInt}
		}
		return SafeInteger{int64(i)}
	case uint8:
		return SafeInteger{int64(i)}
	case uint16:
		return SafeInteger{int64(i)}
	case uint32:
		return SafeInteger{int64(i)}
	case uint64:
		if i > math.MaxInt64 {
			return SafeInteger{invalidSafeInt}
		}
		return SafeInteger{int64(i)}
	case uintptr:
		if uint64(i) > math.MaxInt64 {
			return SafeInteger{invalidSafeInt}
		}
		return SafeInteger{int64(i)}
	case float32:
		if minValidFloat32 <= i && i <= maxValidFloat32 {
			return SafeInteger{int64(i)}
		}
		return SafeInteger{invalidSafeInt}
	case float64:
		if minValidFloat64 <= i && i <= maxValidFloat64 {
			return SafeInteger{int64(i)}
		}
		return SafeInteger{invalidSafeInt}
	default:
		panic("unreachable")
	}
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

func (si SafeInteger) Int64() (int64, bool) {
	if !si.Valid() {
		return 0, false
	}
	return si.value, true
}

func (si SafeInteger) Uint() (uint, bool) {
	i64, ok := si.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || math.MaxUint < uint64(i64) {
		return 0, false
	}
	return uint(i64), true
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

func (i SafeInteger) Uint16() (uint16, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || math.MaxUint16 < i64 {
		return 0, false
	}
	return uint16(i64), true
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

//go:inline
func (si SafeInteger) Valid() bool {
	return si.value != invalidSafeInt
}

func SafeNeg[I Integer | SafeInteger](i I) SafeInteger {
	si := SafeInt(i)
	// Note: as invalidSafeInt == math.MinInt64 and as -math.MinInt64 ==
	// math.MinInt64 within the space of int64s, negation is always valid.
	return SafeInteger{-si.value}
}

func SafeAdd[A, B Integer | SafeInteger](a A, b B) SafeInteger {
	sa, sb := SafeInt(a), SafeInt(b)
	if !sa.Valid() || !sb.Valid() {
		return SafeInteger{invalidSafeInt}
	}

	ret := sa.value + sb.value
	if sameSign(sa.value, sb.value) && !sameSign(ret, sa.value) {
		// An overflow occurred.
		return SafeInteger{invalidSafeInt}
	}
	return SafeInteger{ret}
}

func SafeSub[A, B Integer | SafeInteger](a A, b B) SafeInteger {
	return SafeAdd(a, SafeNeg(b))
}

func SafeMul[A, B Integer | SafeInteger](a A, b B) SafeInteger {
	sa, sb := SafeInt(a), SafeInt(b)
	if !sa.Valid() || !sb.Valid() {
		return SafeInteger{invalidSafeInt}
	}

	if sa.value == 0 {
		return SafeInteger{0}
	}
	if ab := sa.value * sb.value; ab/sa.value == sb.value {
		// No overflow occurred.
		return SafeInteger{ab}
	}
	return SafeInteger{invalidSafeInt}
}

func SafeDiv[A, B Integer | SafeInteger](a A, b B) SafeInteger {
	sa, sb := SafeInt(a), SafeInt(b)
	if !sa.Valid() || !sb.Valid() {
		return SafeInteger{invalidSafeInt}
	}

	if sb.value == 0 {
		return SafeInteger{invalidSafeInt}
	}
	return SafeInteger{sa.value / sb.value}
}

func SafeMax[I Integer | SafeInteger](i I, is ...I) SafeInteger {
	si := SafeInt(i)
	if !si.Valid() {
		return SafeInteger{invalidSafeInt}
	}

	max := si
	for _, i := range is {
		si := SafeInt(i)
		if !si.Valid() {
			return SafeInteger{invalidSafeInt}
		}
		if si.value > max.value {
			max = si
		}
	}
	return max
}

func SafeMin[I Integer | SafeInteger](i I, is ...I) SafeInteger {
	si := SafeInt(i)
	if !si.Valid() {
		return SafeInteger{invalidSafeInt}
	}

	min := si
	for _, i := range is {
		si := SafeInt(i)
		if !si.Valid() {
			return SafeInteger{invalidSafeInt}
		}
		if si.value < min.value {
			min = si
		}
	}
	return min
}

//go:inline
func sameSign(a, b int64) bool {
	return a^b >= 0
}
