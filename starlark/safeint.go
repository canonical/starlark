package starlark

import (
	"fmt"
	"math"
)

// SafeInt represent an overflow-safe integer.
// It is always safe to convert a signed int to
// a SafeInt. For unsigned it is possible to use
// the SafeUint64 function that purpose.
type SafeInt int64

// Marker value to indicate that an overflow has occurred,
// NB: As this value is equal to MinInt64, the space of valid safe integers is
// closed under negation.
const invalidSafeInt = SafeInt(math.MinInt64)

func NewSafeInt(value interface{}) SafeInt {
	// Generics could work here, but do we really want to bump Go1.16->1.18 just for this?
	switch value := value.(type) {
	case int64:
		return SafeInt(value)
	case uint64:
		if value > math.MaxInt64 {
			return invalidSafeInt
		}
		return SafeInt(value)
		// TODO(kcza): cover all the other integer types (this is just a proof of concept).
	default:
		panic(fmt.Sprintf("cannot make SafeInt from %T", value))
	}
}

func SafeUint64(u uint64) SafeInt {
	if u > math.MaxInt64 {
		return invalidSafeInt
	}
	return SafeInt(u)
}

func (i SafeInt) Int64() (int64, bool) {
	if i == invalidSafeInt {
		return 0, false
	}
	return int64(i), true
}

func (i SafeInt) Uint64() (uint64, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 {
		return 0, false
	}
	return uint64(i64), true
}

func (i SafeInt) Int() (int, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < math.MinInt || math.MaxInt < i64 {
		return 0, false
	}
	return int(i64), true
}

func (i SafeInt) Uint() (uint, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || math.MaxUint < uint(i64) {
		return 0, false
	}
	return uint(i64), true
}

func (i SafeInt) Int32() (int32, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < math.MinInt32 || math.MaxInt32 < i64 {
		return 0, false
	}
	return int32(i64), true
}

func (i SafeInt) Uint32() (uint32, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || math.MaxUint32 < i64 {
		return 0, false
	}
	return uint32(i64), true
}

func (i SafeInt) Int16() (int16, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < math.MinInt16 || math.MaxInt16 < i64 {
		return 0, false
	}
	return int16(i64), true
}

func (i SafeInt) Uint16() (uint16, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || i64 > math.MaxUint16 {
		return 0, false
	}
	return uint16(i64), true
}

func (i SafeInt) Int8() (int8, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < math.MinInt8 || math.MaxInt8 < i64 {
		return 0, false
	}
	return int8(i64), true
}

func (i SafeInt) Uint8() (uint8, bool) {
	i64, ok := i.Int64()
	if !ok {
		return 0, false
	}
	if i64 < 0 || math.MaxUint8 < i64 {
		return 0, false
	}
	return uint8(i64), true
}

func (a SafeInt) Add(b SafeInt) SafeInt {
	if a == invalidSafeInt || b == invalidSafeInt {
		return invalidSafeInt
	}

	ret := a + b
	if sameSign(a, b) && !sameSign(ret, a) {
		return invalidSafeInt
	}
	return ret
}

func (a SafeInt) Sub(b SafeInt) SafeInt {
	if b == invalidSafeInt || b == invalidSafeInt {
		return invalidSafeInt
	}
	return a.Add(-b)
}

func (a SafeInt) Mul(b SafeInt) SafeInt {
	if a == invalidSafeInt || b == invalidSafeInt {
		return invalidSafeInt
	}
	if a == 0 || b == 0 {
		return 0
	}

	ab := a * b
	if ab/a != b {
		return invalidSafeInt
	}
	return ab
}

func (a SafeInt) Div(b SafeInt) SafeInt {
	if a == invalidSafeInt || b == invalidSafeInt {
		return invalidSafeInt
	}
	return a / b
}

func sameSign(a, b SafeInt) bool {
	return a^b >= 0
}
