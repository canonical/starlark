package starlark_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestSafeIntString(t *testing.T) {
	tests := []struct {
		name     string
		safeInt  starlark.SafeInteger
		expected string
	}{{
		name:     "valid",
		safeInt:  starlark.SafeInt(10),
		expected: "SafeInt(10)",
	}, {
		name:     "invalid",
		safeInt:  starlark.InvalidSafeInt,
		expected: "SafeInt(invalid)",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if repr := test.safeInt.String(); repr != test.expected {
				t.Errorf("incorrect string representation: expected %q but got %q", test.expected, repr)
			}
		})
	}
}

type safeIntRoundtripTest[I starlark.Integer | float64] struct {
	name       string
	value      I
	converter  func(starlark.SafeInteger) (I, bool)
	shouldFail bool
}

func (test *safeIntRoundtripTest[_]) Run(t *testing.T) {
	t.Run(test.name, func(t *testing.T) {
		value, ok := test.converter(starlark.SafeInt(test.value))
		if test.shouldFail && ok {
			t.Errorf("expected failure, got %v", value)
		} else if !test.shouldFail && !ok {
			t.Errorf("expected success, got failure")
		} else if !test.shouldFail && value != test.value {
			t.Errorf("unexpected value: want %v, got %v", test.value, value)
		}
	})
}

func TestSafeIntRoundtrip(t *testing.T) {
	t.Run("SafeInteger", func(t *testing.T) {
		safeInt1 := starlark.SafeInt(1234)
		safeInt2 := starlark.SafeInt(safeInt1)
		if !reflect.DeepEqual(safeInt1, safeInt2) {
			t.Errorf("value not preserved: expected %d but got %d", safeInt1, safeInt2)
		}
	})

	t.Run("int", func(t *testing.T) {
		tests := []safeIntRoundtripTest[int]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Int,
		}, {
			name:      "negative",
			value:     -100,
			converter: starlark.SafeInteger.Int,
		}, {
			name:      "max",
			value:     math.MaxInt,
			converter: starlark.SafeInteger.Int,
		}, {
			name:      "-max",
			value:     -math.MaxInt,
			converter: starlark.SafeInteger.Int,
		}, {
			name:       "min",
			value:      math.MinInt,
			converter:  starlark.SafeInteger.Int,
			shouldFail: math.MinInt == math.MinInt64,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("int8", func(t *testing.T) {
		tests := []safeIntRoundtripTest[int8]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Int8,
		}, {
			name:      "negative",
			value:     -100,
			converter: starlark.SafeInteger.Int8,
		}, {
			name:      "max",
			value:     math.MaxInt8,
			converter: starlark.SafeInteger.Int8,
		}, {
			name:      "min",
			value:     math.MinInt8,
			converter: starlark.SafeInteger.Int8,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("int16", func(t *testing.T) {
		tests := []safeIntRoundtripTest[int16]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Int16,
		}, {
			name:      "negative",
			value:     -100,
			converter: starlark.SafeInteger.Int16,
		}, {
			name:      "max",
			value:     math.MaxInt16,
			converter: starlark.SafeInteger.Int16,
		}, {
			name:      "min",
			value:     math.MinInt16,
			converter: starlark.SafeInteger.Int16,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("int32", func(t *testing.T) {
		tests := []safeIntRoundtripTest[int32]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Int32,
		}, {
			name:      "negative",
			value:     -100,
			converter: starlark.SafeInteger.Int32,
		}, {
			name:      "max",
			value:     math.MaxInt32,
			converter: starlark.SafeInteger.Int32,
		}, {
			name:      "min",
			value:     math.MinInt32,
			converter: starlark.SafeInteger.Int32,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("int64", func(t *testing.T) {
		tests := []safeIntRoundtripTest[int64]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Int64,
		}, {
			name:      "negative",
			value:     -100,
			converter: starlark.SafeInteger.Int64,
		}, {
			name:      "max",
			value:     math.MaxInt64,
			converter: starlark.SafeInteger.Int64,
		}, {
			name:      "-max",
			value:     -math.MaxInt64,
			converter: starlark.SafeInteger.Int64,
		}, {
			name:       "min",
			value:      math.MinInt64,
			converter:  starlark.SafeInteger.Int64,
			shouldFail: true,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint", func(t *testing.T) {
		tests := []safeIntRoundtripTest[uint]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Uint,
		}, {
			name:       "max",
			value:      math.MaxUint,
			converter:  starlark.SafeInteger.Uint,
			shouldFail: math.MaxUint == math.MaxUint64,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint8", func(t *testing.T) {
		tests := []safeIntRoundtripTest[uint8]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Uint8,
		}, {
			name:      "max",
			value:     math.MaxUint8,
			converter: starlark.SafeInteger.Uint8,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint8,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint16", func(t *testing.T) {
		tests := []safeIntRoundtripTest[uint16]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Uint16,
		}, {
			name:      "max",
			value:     math.MaxUint16,
			converter: starlark.SafeInteger.Uint16,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint16,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint32", func(t *testing.T) {
		tests := []safeIntRoundtripTest[uint32]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Uint32,
		}, {
			name:      "max",
			value:     math.MaxUint32,
			converter: starlark.SafeInteger.Uint32,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint32,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint64", func(t *testing.T) {
		tests := []safeIntRoundtripTest[uint64]{{
			name:      "positive",
			value:     100,
			converter: starlark.SafeInteger.Uint64,
		}, {
			name:       "max",
			value:      math.MaxUint64,
			converter:  starlark.SafeInteger.Uint64,
			shouldFail: true,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint64,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("float64", func(t *testing.T) {
		const (
			qNaN = 0x7FF8000000000001
			sNaN = 0x7ff0000000000001
		)
		asFloat := func(si starlark.SafeInteger) (float64, bool) {
			if i, ok := si.Int64(); ok {
				return float64(i), ok
			}
			return 0, false
		}

		tests := []safeIntRoundtripTest[float64]{{
			name:      "positive",
			value:     100,
			converter: asFloat,
		}, {
			name:      "negative",
			value:     -100,
			converter: asFloat,
		}, {
			name:      "zero",
			value:     0,
			converter: asFloat,
		}, {
			name:       "int-max",
			value:      math.MaxUint64,
			converter:  asFloat,
			shouldFail: true,
		}, {
			name:       "int-min",
			value:      -math.MaxUint64,
			converter:  asFloat,
			shouldFail: true,
		}, {
			name:       "float-max",
			value:      math.MaxFloat64,
			converter:  asFloat,
			shouldFail: true,
		}, {
			name:       "float-min",
			value:      -math.MaxFloat64,
			converter:  asFloat,
			shouldFail: true,
		}, {
			name:       "+inf",
			value:      math.Inf(1),
			converter:  asFloat,
			shouldFail: true,
		}, {
			name:       "-inf",
			value:      math.Inf(-1),
			converter:  asFloat,
			shouldFail: true,
		}, {
			name:       "NaN",
			value:      math.NaN(),
			converter:  asFloat,
			shouldFail: true,
		}, {
			name:       "qNaN",
			value:      math.Float64frombits(qNaN),
			converter:  asFloat,
			shouldFail: true,
		}, {
			name:       "sNaN",
			value:      math.Float64frombits(sNaN),
			converter:  asFloat,
			shouldFail: true,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})
}

func TestSafeIntFloatTruncation(t *testing.T) {
	safeInt := starlark.SafeInt(1.2)
	if value, ok := safeInt.Int64(); !ok {
		t.Errorf("expected success, got failure")
	} else if value != 1 {
		t.Errorf("unexpected value: want %v, got %v", 1, value)
	}
}

func TestSafeIntUintTruncation(t *testing.T) {
	if math.MaxUint == math.MaxUint64 {
		// The truncation issue does not occur on 64-bit platforms.
		return
	}

	// input is a value which would cause .Uint() to return !ok, unless while
	// running on a 32-bit machine, it is first truncated to an int/uint.
	const input = uint64(math.MaxInt64 &^ (1 << 31))

	_, ok := starlark.SafeInt(input).Uint()
	if ok {
		t.Errorf("expected conversion to fail")
	}
}

type safeIntInvalidConversionTest[I starlark.Integer] struct {
	name          string
	value         starlark.SafeInteger
	converter     func(starlark.SafeInteger) (I, bool)
	shouldSucceed bool
}

func (test *safeIntInvalidConversionTest[_]) Run(t *testing.T) {
	t.Run(test.name, func(t *testing.T) {
		value, ok := test.converter(test.value)
		if !test.shouldSucceed && ok {
			t.Errorf("expected failure, got %d", value)
		} else if test.shouldSucceed && !ok {
			t.Errorf("expected success, got failure from value %v", test.value)
		}
	})
}

func TestSafeIntInvalidConversions(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[int]{{
			name:          "too-large",
			value:         starlark.SafeInt(int64(math.MaxInt64)),
			converter:     starlark.SafeInteger.Int,
			shouldSucceed: math.MaxInt == math.MaxInt64,
		}, {
			name:          "too-small",
			value:         starlark.SafeInt(int64(math.MinInt64 + 1)),
			converter:     starlark.SafeInteger.Int,
			shouldSucceed: math.MinInt == math.MinInt64,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("int8", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[int8]{{
			name:      "too-large",
			value:     starlark.SafeInt(math.MaxInt8 + 1),
			converter: starlark.SafeInteger.Int8,
		}, {
			name:      "too-small",
			value:     starlark.SafeInt(math.MinInt8 - 1),
			converter: starlark.SafeInteger.Int8,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("int16", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[int16]{{
			name:      "too-large",
			value:     starlark.SafeInt(math.MaxInt16 + 1),
			converter: starlark.SafeInteger.Int16,
		}, {
			name:      "too-small",
			value:     starlark.SafeInt(math.MinInt16 - 1),
			converter: starlark.SafeInteger.Int16,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("int32", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[int32]{{
			name:      "too-large",
			value:     starlark.SafeInt(int64(math.MaxInt32 + 1)),
			converter: starlark.SafeInteger.Int32,
		}, {
			name:      "too-small",
			value:     starlark.SafeInt(int64(math.MinInt32 - 1)),
			converter: starlark.SafeInteger.Int32,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[uint]{{
			name:      "too-large",
			value:     starlark.SafeInt(uint64(math.MaxUint64)),
			converter: starlark.SafeInteger.Uint,
		}, {
			name:      "negative",
			value:     starlark.SafeInt(-100),
			converter: starlark.SafeInteger.Uint,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint8", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[uint8]{{
			name:      "too-large",
			value:     starlark.SafeInt(math.MaxUint8 + 1),
			converter: starlark.SafeInteger.Uint8,
		}, {
			name:      "negative",
			value:     starlark.SafeInt(-1),
			converter: starlark.SafeInteger.Uint8,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint16", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[uint16]{{
			name:      "too-large",
			value:     starlark.SafeInt(math.MaxUint16 + 1),
			converter: starlark.SafeInteger.Uint16,
		}, {
			name:      "negative",
			value:     starlark.SafeInt(-1),
			converter: starlark.SafeInteger.Uint16,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint32", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[uint32]{{
			name:      "too-large",
			value:     starlark.SafeInt(int64(math.MaxUint32 + 1)),
			converter: starlark.SafeInteger.Uint32,
		}, {
			name:      "negative",
			value:     starlark.SafeInt(-1),
			converter: starlark.SafeInteger.Uint32,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("uint64", func(t *testing.T) {
		tests := []safeIntInvalidConversionTest[uint64]{{
			name:      "negative",
			value:     starlark.SafeInt(-1),
			converter: starlark.SafeInteger.Uint64,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})
}

func TestInvalidSafeIntConversions(t *testing.T) {
	tests := []struct {
		name      string
		converter func(si starlark.SafeInteger) (any, bool)
	}{{
		name:      "int",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Int() },
	}, {
		name:      "int8",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Int8() },
	}, {
		name:      "int16",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Int16() },
	}, {
		name:      "int32",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Int32() },
	}, {
		name:      "int64",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Int64() },
	}, {
		name:      "uint",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Uint() },
	}, {
		name:      "uint8",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Uint8() },
	}, {
		name:      "uint16",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Uint16() },
	}, {
		name:      "uint32",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Uint32() },
	}, {
		name:      "uint64",
		converter: func(si starlark.SafeInteger) (any, bool) { return si.Uint64() },
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := test.converter(starlark.InvalidSafeInt); ok {
				t.Error("conversion unexpectedly succeeded")
			}
		})
	}
}

func TestSafeIntValid(t *testing.T) {
	validSafeInt := starlark.SafeInt(10)
	if !validSafeInt.Valid() {
		t.Error("valid SafeInteger reported as invalid")
	}

	if starlark.InvalidSafeInt.Valid() {
		t.Error("invalid SafeInteger reported as valid")
	}
}

func TestSafeNeg(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected starlark.SafeInteger
	}{{
		name:     "zero",
		input:    0,
		expected: starlark.SafeInt(0),
	}, {
		name:     "valid-non-zero",
		input:    100,
		expected: starlark.SafeInt(-100),
	}, {
		name:     "invalid",
		input:    starlark.InvalidSafeInt,
		expected: starlark.InvalidSafeInt,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var negated starlark.SafeInteger
			switch input := test.input.(type) {
			case int:
				negated = starlark.SafeNeg(input)
			case starlark.SafeInteger:
				negated = starlark.SafeNeg(input)
			default:
				panic("unreachable")
			}
			if !reflect.DeepEqual(negated, test.expected) {
				t.Errorf("incorrect value: expected %v but got %v", test.expected, negated)
			}
		})
	}
}

func TestSafeAdd(t *testing.T) {
	tests := []struct {
		name     string
		sum      starlark.SafeInteger
		expected starlark.SafeInteger
	}{{
		name:     "valid",
		sum:      starlark.SafeAdd(100, -200),
		expected: starlark.SafeInt(-100),
	}, {
		name:     "invalid-first",
		sum:      starlark.SafeAdd(starlark.InvalidSafeInt, 100),
		expected: starlark.InvalidSafeInt,
	}, {
		name:     "invalid-second",
		sum:      starlark.SafeAdd(100, starlark.InvalidSafeInt),
		expected: starlark.InvalidSafeInt,
	}, {
		name:     "overflow",
		sum:      starlark.SafeAdd(math.MaxInt64, math.MaxInt64),
		expected: starlark.InvalidSafeInt,
	}, {
		name:     "underflow",
		sum:      starlark.SafeAdd(math.MinInt64, math.MinInt64),
		expected: starlark.InvalidSafeInt,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.sum != test.expected {
				t.Errorf("incorrect sum: expected %v but got %v", test.expected, test.sum)
			}
		})
	}
}

func TestSafeSub(t *testing.T) {
	// TODO(kcza): implement this.
	t.Skip()
}

func TestSafeMul(t *testing.T) {
	tests := []struct {
		name     string
		product  starlark.SafeInteger
		expected starlark.SafeInteger
	}{{
		name:     "valid",
		product:  starlark.SafeMul(100, 100),
		expected: starlark.SafeInt(10000),
	}, {
		name:     "by-zero-first",
		product:  starlark.SafeMul(100, 0),
		expected: starlark.SafeInt(0),
	}, {
		name:     "by-zero-second",
		product:  starlark.SafeMul(0, 100),
		expected: starlark.SafeInt(0),
	}, {
		name:     "invalid-first",
		product:  starlark.SafeMul(starlark.InvalidSafeInt, 100),
		expected: starlark.InvalidSafeInt,
	}, {
		name:     "invalid-second",
		product:  starlark.SafeMul(100, starlark.InvalidSafeInt),
		expected: starlark.InvalidSafeInt,
	}, {
		name:     "overflow",
		product:  starlark.SafeMul(math.MaxInt64/2, 4),
		expected: starlark.InvalidSafeInt,
	}, {
		name:     "underflow",
		product:  starlark.SafeMul(math.MinInt64/2, 4),
		expected: starlark.InvalidSafeInt,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.product != test.expected {
				t.Errorf("incorrect sum: expected %v but got %v", test.expected, test.product)
			}
		})
	}
}

func TestSafeDiv(t *testing.T) {
	tests := []struct {
		name     string
		quotient starlark.SafeInteger
		expected starlark.SafeInteger
	}{{
		name:     "positive/positive",
		quotient: starlark.SafeDiv(1000, 100),
		expected: starlark.SafeInt(10),
	}, {
		name:     "positive/negative",
		quotient: starlark.SafeDiv(1000, -100),
		expected: starlark.SafeInt(-10),
	}, {
		name:     "invalid-first",
		quotient: starlark.SafeDiv(starlark.InvalidSafeInt, 100),
		expected: starlark.InvalidSafeInt,
	}, {
		name:     "invalid-second",
		quotient: starlark.SafeDiv(1000, starlark.InvalidSafeInt),
		expected: starlark.InvalidSafeInt,
	}, {
		name:     "divide-by-zero",
		quotient: starlark.SafeDiv(1000, 0),
		expected: starlark.InvalidSafeInt,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.quotient != test.expected {
				t.Errorf("incorrect result: expected %v but got %v", test.expected, test.quotient)
			}
		})
	}
}
