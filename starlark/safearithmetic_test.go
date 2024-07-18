package starlark_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestSafeAdd(t *testing.T) {
	tests := []struct {
		name         string
		a, b, expect int
	}{{
		name:   "0+0",
		a:      0,
		b:      0,
		expect: 0,
	}, {
		name:   "MaxInt+MaxInt",
		a:      math.MaxInt,
		b:      math.MaxInt,
		expect: math.MaxInt,
	}, {
		name:   "MaxInt+MinInt",
		a:      math.MaxInt,
		b:      math.MinInt,
		expect: math.MaxInt,
	}, {
		name:   "MinInt+MaxInt",
		a:      math.MinInt,
		b:      math.MaxInt,
		expect: math.MinInt,
	}, {
		name:   "MinInt+MinInt",
		a:      math.MinInt,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
		name:   "100+MinInt",
		a:      100,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
		name:   "-100+MaxInt",
		a:      -100,
		b:      math.MaxInt,
		expect: math.MaxInt,
	}}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d + %d", test.a, test.b), func(t *testing.T) {
			if actual := starlark.SafeAdd(test.a, test.b); actual != test.expect {
				t.Errorf("incorrect result: expected %d but got %d", test.expect, actual)
			}
		})
	}
}

func TestSafeAdd64(t *testing.T) {
	tests := []struct {
		name         string
		a, b, expect int64
	}{{
		name:   "9+16",
		a:      9,
		b:      16,
		expect: 25,
	}, {
		name:   "MaxInt64+MaxInt64",
		a:      math.MaxInt64,
		b:      math.MaxInt64,
		expect: math.MaxInt64,
	}, {
		name:   "MaxInt64+MinInt64",
		a:      math.MaxInt64,
		b:      math.MinInt64,
		expect: math.MaxInt64,
	}, {
		name:   "MinInt64+MaxInt64",
		a:      math.MinInt64,
		b:      math.MaxInt64,
		expect: math.MinInt64,
	}, {
		name:   "MinInt64+MinInt64",
		a:      math.MinInt64,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
		name:   "100+MinInt64",
		a:      100,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
		name:   "-100+MaxInt64",
		a:      -100,
		b:      math.MaxInt64,
		expect: math.MaxInt64,
	}}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d + %d", test.a, test.b), func(t *testing.T) {
			if actual := starlark.SafeAdd64(test.a, test.b); actual != test.expect {
				t.Errorf("incorrect result: expected %d but got %d", test.expect, actual)
			}
		})
	}
}

func TestSafeMul(t *testing.T) {
	tests := []struct {
		name         string
		a, b, expect int
	}{{
		name:   "4+25",
		a:      4,
		b:      25,
		expect: 100,
	}, {
		name:   "100*-100",
		a:      100,
		b:      -100,
		expect: -10000,
	}, {
		name:   "8 * MinInt64/16",
		a:      8,
		b:      math.MinInt / 16,
		expect: math.MinInt / 2,
	}, {
		name:   "MaxInt+MaxInt",
		a:      math.MaxInt,
		b:      math.MaxInt,
		expect: math.MaxInt,
	}, {
		name:   "MaxInt+MinInt",
		a:      math.MaxInt,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
		name:   "MinInt+MaxInt",
		a:      math.MinInt,
		b:      math.MaxInt,
		expect: math.MinInt,
	}, {
		name:   "MinInt+MinInt",
		a:      math.MinInt,
		b:      math.MinInt,
		expect: math.MaxInt,
	}, {
		name:   "100+MinInt",
		a:      100,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
		name:   "-100+MaxInt",
		a:      -100,
		b:      math.MaxInt,
		expect: math.MinInt,
	}}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d + %d", test.a, test.b), func(t *testing.T) {
			if actual := starlark.SafeMul(test.a, test.b); actual != test.expect {
				t.Errorf("incorrect result: expected %d but got %d", test.expect, actual)
			}
		})
	}
}

func TestSafeMul64(t *testing.T) {
	tests := []struct {
		name         string
		a, b, expect int64
	}{{
		name:   "4+25",
		a:      4,
		b:      25,
		expect: 100,
	}, {
		name:   "100*-100",
		a:      100,
		b:      -100,
		expect: -10000,
	}, {
		name:   "8 * MinInt64/16",
		a:      8,
		b:      math.MinInt64 / 16,
		expect: math.MinInt64 / 2,
	}, {
		name:   "MaxInt64+MaxInt64",
		a:      math.MaxInt64,
		b:      math.MaxInt64,
		expect: math.MaxInt64,
	}, {
		name:   "MaxInt64+MinInt64",
		a:      math.MaxInt64,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
		name:   "MinInt64+MaxInt64",
		a:      math.MinInt64,
		b:      math.MaxInt64,
		expect: math.MinInt64,
	}, {
		name:   "MinInt64+MinInt64",
		a:      math.MinInt64,
		b:      math.MinInt64,
		expect: math.MaxInt64,
	}, {
		name:   "100+MinInt64",
		a:      100,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
		name:   "-100+MaxInt64",
		a:      -100,
		b:      math.MaxInt64,
		expect: math.MinInt64,
	}}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d + %d", test.a, test.b), func(t *testing.T) {
			if actual := starlark.SafeMul64(test.a, test.b); actual != test.expect {
				t.Errorf("incorrect result: expected %d but got %d", test.expect, actual)
			}
		})
	}
}
