package starlark_test

import (
	"math"
	"testing"

	"github.com/canonical/starlark/starlark"
)

type safeIntTest struct {
	name       string
	input      starlark.SafeInteger
	expected   interface{}
	shouldFail bool
}

func runSafeIntConversionTest[Int starlark.Integer](t *testing.T, convert func(starlark.SafeInteger) (Int, bool), tests []safeIntTest) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, ok := convert(test.input)
			if test.shouldFail && ok {
				t.Errorf("expected failure, got %d", value)
			} else if !test.shouldFail && !ok {
				t.Errorf("expected success, got failure")
			} else if !test.shouldFail && value != test.expected {
				t.Errorf("unexpected value: want %d, got %d", test.expected, value)
			}
		})
	}
}

func TestSafeIntConversion(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Int, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(int(100)),
			expected: int(100),
		}, {
			name:     "negative",
			input:    starlark.SafeInt(int(-100)),
			expected: int(-100),
		}, {
			name:     "max",
			input:    starlark.SafeInt(int(math.MaxInt)),
			expected: int(math.MaxInt),
		}, {
			name:     "-max",
			input:    starlark.SafeInt(int(-math.MaxInt)),
			expected: int(-math.MaxInt),
		}, {
			name:       "min",
			input:      starlark.SafeInt(int(math.MinInt)),
			expected:   int(math.MinInt),
			shouldFail: math.MinInt == math.MinInt64,
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("int64", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Int64, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(int64(100)),
			expected: int64(100),
		}, {
			name:     "negative",
			input:    starlark.SafeInt(int64(-100)),
			expected: int64(-100),
		}, {
			name:     "max",
			input:    starlark.SafeInt(int64(math.MaxInt64)),
			expected: int64(math.MaxInt64),
		}, {
			name:     "-max",
			input:    starlark.SafeInt(int64(-math.MaxInt64)),
			expected: int64(-math.MaxInt64),
		}, {
			name:       "min",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("int32", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Int32, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(int32(100)),
			expected: int32(100),
		}, {
			name:     "negative",
			input:    starlark.SafeInt(int32(-100)),
			expected: int32(-100),
		}, {
			name:     "max",
			input:    starlark.SafeInt(int32(math.MaxInt32)),
			expected: int32(math.MaxInt32),
		}, {
			name:     "min",
			input:    starlark.SafeInt(int32(math.MinInt32)),
			expected: int32(math.MinInt32),
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("int16", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Int16, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(100),
			expected: int16(100),
		}, {
			name:     "negative",
			input:    starlark.SafeInt(int16(-100)),
			expected: int16(-100),
		}, {
			name:     "max",
			input:    starlark.SafeInt(int16(math.MaxInt16)),
			expected: int16(math.MaxInt16),
		}, {
			name:     "min",
			input:    starlark.SafeInt(int16(math.MinInt16)),
			expected: int16(math.MinInt16),
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("int8", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Int8, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(int8(100)),
			expected: int8(100),
		}, {
			name:     "negative",
			input:    starlark.SafeInt(int8(-100)),
			expected: int8(-100),
		}, {
			name:     "max",
			input:    starlark.SafeInt(int8(math.MaxInt8)),
			expected: int8(math.MaxInt8),
		}, {
			name:     "min",
			input:    starlark.SafeInt(int8(math.MinInt8)),
			expected: int8(math.MinInt8),
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("uint", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Uint, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(uint(100)),
			expected: uint(100),
		}, {
			name:       "negative",
			input:      starlark.SafeInt(-100),
			shouldFail: true,
		}, {
			name:       "max",
			input:      starlark.SafeInt(uint(math.MaxUint)),
			expected:   uint(math.MaxUint),
			shouldFail: math.MaxUint == math.MaxUint64,
		}, {
			name:     "zero",
			input:    starlark.SafeInt(uint(0)),
			expected: uint(0),
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("uint64", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Uint64, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(uint64(100)),
			expected: uint64(100),
		}, {
			name:       "negative",
			input:      starlark.SafeInt(-100),
			shouldFail: true,
		}, {
			name:       "max",
			input:      starlark.SafeInt(uint64(math.MaxUint64)),
			shouldFail: true,
		}, {
			name:     "zero",
			input:    starlark.SafeInt(uint64(0)),
			expected: uint64(0),
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("uint32", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Uint32, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(uint32(100)),
			expected: uint32(100),
		}, {
			name:       "negative",
			input:      starlark.SafeInt(-100),
			shouldFail: true,
		}, {
			name:     "max",
			input:    starlark.SafeInt(uint32(math.MaxUint32)),
			expected: uint32(math.MaxUint32),
		}, {
			name:     "zero",
			input:    starlark.SafeInt(uint32(0)),
			expected: uint32(0),
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("uint16", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Uint16, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(uint16(100)),
			expected: uint16(100),
		}, {
			name:       "negative",
			input:      starlark.SafeInt(-100),
			shouldFail: true,
		}, {
			name:     "max",
			input:    starlark.SafeInt(uint16(math.MaxUint16)),
			expected: uint16(math.MaxUint16),
		}, {
			name:     "zero",
			input:    starlark.SafeInt(uint16(0)),
			expected: uint16(0),
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})

	t.Run("uint8", func(t *testing.T) {
		runSafeIntConversionTest(t, starlark.SafeInteger.Uint8, []safeIntTest{{
			name:     "positive",
			input:    starlark.SafeInt(uint8(100)),
			expected: uint8(100),
		}, {
			name:       "negative",
			input:      starlark.SafeInt(-100),
			shouldFail: true,
		}, {
			name:     "max",
			input:    starlark.SafeInt(uint8(math.MaxUint8)),
			expected: uint8(math.MaxUint8),
		}, {
			name:     "zero",
			input:    starlark.SafeInt(uint8(0)),
			expected: uint8(0),
		}, {
			name:       "invalid",
			input:      starlark.SafeInt(int64(math.MinInt64)),
			shouldFail: true,
		}})
	})
}
