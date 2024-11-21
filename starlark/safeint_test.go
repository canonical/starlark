package starlark_test

import (
	"math"
	"testing"

	"github.com/canonical/starlark/starlark"
)

type safeIntRoundtripTest[I starlark.Integer] struct {
	name       string
	value      I
	converter  func(starlark.SafeInteger) (I, bool)
	shouldFail bool
}

func (test *safeIntRoundtripTest[_]) Run(t *testing.T) {
	t.Run(test.name, func(t *testing.T) {
		value, ok := test.converter(starlark.SafeInt(test.value))
		if test.shouldFail && ok {
			t.Errorf("expected failure, got %d", value)
		} else if !test.shouldFail && !ok {
			t.Errorf("expected success, got failure")
		} else if !test.shouldFail && value != test.value {
			t.Errorf("unexpected value: want %d, got %d", test.value, value)
		}
	})
}

func TestSafeIntRoundtrip(t *testing.T) {
	t.Run("SafeInteger", func(t *testing.T) {
		const expected = 1234
		safeInt := starlark.SafeInt(starlark.SafeInt(expected))
		if converted, ok := safeInt.Int(); !ok {
			t.Error("unexpected converson failure")
		} else if converted != expected {
			t.Errorf("value not preserved: expected %d but got %d", expected, converted)
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
			// }, {
			// 	name:       "invalid",
			// 	input:      math.MinInt64,
			// 	extractor:  starlark.SafeInteger.Int8,
			// 	shouldFail: true,
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
			// }, {
			// 	name:       "invalid",
			// 	input:      starlark.SafeInt(int64(math.MinInt64)),
			// 	extractor: staralrk.SafeInteger.Int16,
			// 	shouldFail: true,
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
			// }, {
			// 	name:       "invalid",
			// 	input:      math.MinInt64,
			// 	extractor:  starlark.SafeInteger.Int32,
			// 	shouldFail: true,
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
			// }, {
			// 	name:       "negative",
			// 	input:      -100,
			// 	extractor:  starlark.SafeInteger.Uint,
			// 	shouldFail: true,
		}, {
			name:       "max",
			value:      math.MaxUint,
			converter:  starlark.SafeInteger.Uint,
			shouldFail: math.MaxUint == math.MaxUint64,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint,
			// }, {
			// 	name:       "invalid",
			// 	input:      math.MinInt64,
			// 	extractor:  starlark.SafeInteger.Uint,
			// 	shouldFail: true,
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
			// }, {
			// 	name:       "negative",
			// 	input:      -100,
			// 	extractor:  starlark.SafeInteger.Uint8,
			// 	shouldFail: true,
		}, {
			name:      "max",
			value:     math.MaxUint8,
			converter: starlark.SafeInteger.Uint8,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint8,
			// }, {
			// 	name:       "invalid",
			// 	input:      math.MinInt64,
			// 	extractor:  starlark.SafeInteger.Uint8,
			// 	shouldFail: true,
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
			// }, {
			// 	name:       "negative",
			// 	input:      -100,
			// 	extractor:  starlark.SafeInteger.Uint16,
			// 	shouldFail: true,
		}, {
			name:      "max",
			value:     math.MaxUint16,
			converter: starlark.SafeInteger.Uint16,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint16,
			// }, {
			// 	name:       "invalid",
			// 	input:      math.MinInt64,
			// 	extractor:  starlark.SafeInteger.Uint16,
			// 	shouldFail: true,
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
			// }, {
			// 	name:       "negative",
			// 	input:      -100,
			// 	extractor:  starlark.SafeInteger.Uint32,
			// 	shouldFail: true,
		}, {
			name:      "max",
			value:     math.MaxUint32,
			converter: starlark.SafeInteger.Uint32,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint32,
			// }, {
			// 	name:       "invalid",
			// 	input:      math.MinInt64,
			// 	extractor:  starlark.SafeInteger.Uint32,
			// 	shouldFail: true,
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
			// }, {
			// 	name:       "negative",
			// 	input:      -100,
			// 	extractor:  starlark.SafeInteger.Uint64,
			// 	shouldFail: true,
		}, {
			name:       "max",
			value:      math.MaxUint64,
			converter:  starlark.SafeInteger.Uint64,
			shouldFail: true,
		}, {
			name:      "zero",
			value:     0,
			converter: starlark.SafeInteger.Uint64,
			// }, {
			// 	name:       "invalid",
			// 	input:      math.MinInt64,
			// 	extractor:  starlark.SafeInteger.Uint64,
			// 	shouldFail: true,
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})
}

type safeIntConversionBoundTest[I starlark.Integer] struct {
	name          string
	value         starlark.SafeInteger
	converter     func(starlark.SafeInteger) (I, bool)
	shouldSucceed bool
}

func (test *safeIntConversionBoundTest[_]) Run(t *testing.T) {
	t.Run(test.name, func(t *testing.T) {
		value, ok := test.converter(test.value)
		if !test.shouldSucceed && ok {
			t.Errorf("expected failure, got %d", value)
		} else if test.shouldSucceed && !ok {
			t.Errorf("expected success, got failure from value %v", test.value)
		}
	})
}

func TestSafeIntConversionBounds(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		tests := []safeIntConversionBoundTest[int]{{
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
		tests := []safeIntConversionBoundTest[int8]{{
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
		tests := []safeIntConversionBoundTest[int16]{{
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
		tests := []safeIntConversionBoundTest[int32]{{
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
		tests := []safeIntConversionBoundTest[uint]{{
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
		tests := []safeIntConversionBoundTest[uint8]{{
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
		tests := []safeIntConversionBoundTest[uint16]{{
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
		tests := []safeIntConversionBoundTest[uint32]{{
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
		tests := []safeIntConversionBoundTest[uint64]{{
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
		converter func(si *starlark.SafeInteger) (any, bool)
	}{{
		name:      "int",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Int() },
	}, {
		name:      "int8",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Int8() },
	}, {
		name:      "int16",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Int16() },
	}, {
		name:      "int32",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Int32() },
	}, {
		name:      "int64",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Int64() },
	}, {
		name:      "uint",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Uint() },
	}, {
		name:      "uint8",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Uint8() },
	}, {
		name:      "uint16",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Uint16() },
	}, {
		name:      "uint32",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Uint32() },
	}, {
		name:      "uint64",
		converter: func(si *starlark.SafeInteger) (any, bool) { return si.Uint64() },
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := test.converter(starlark.InvalidSafeInt); ok {
				t.Error("conversion unexpectedly succeeded")
			}
		})
	}
}
