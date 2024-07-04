package starlark_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestSafeAdd(t *testing.T) {
	tests := []struct{ a, b, expect int }{{
		a:      0,
		b:      0,
		expect: 0,
	}, {
		a:      math.MaxInt,
		b:      math.MaxInt,
		expect: math.MaxInt,
	}, {
		a:      math.MaxInt,
		b:      math.MinInt,
		expect: math.MaxInt,
	}, {
		a:      math.MinInt,
		b:      math.MaxInt,
		expect: math.MinInt,
	}, {
		a:      math.MinInt,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
		a:      100,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
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
	tests := []struct{ a, b, expect int64 }{{
		a:      9,
		b:      16,
		expect: 25,
	}, {
		a:      math.MaxInt64,
		b:      math.MaxInt64,
		expect: math.MaxInt64,
	}, {
		a:      math.MaxInt64,
		b:      math.MinInt64,
		expect: math.MaxInt64,
	}, {
		a:      math.MinInt64,
		b:      math.MaxInt64,
		expect: math.MinInt64,
	}, {
		a:      math.MinInt64,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
		a:      100,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
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
		a, b, expect int
	}{{
		a:      4,
		b:      25,
		expect: 100,
	}, {
		a:      math.MaxInt,
		b:      math.MaxInt,
		expect: math.MaxInt,
	}, {
		a:      math.MaxInt,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
		a:      math.MinInt,
		b:      math.MaxInt,
		expect: math.MinInt,
	}, {
		a:      math.MinInt,
		b:      math.MinInt,
		expect: math.MaxInt,
	}, {
		a:      100,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
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
		a, b, expect int64
	}{{
		a:      4,
		b:      25,
		expect: 100,
	}, {
		a:      math.MaxInt64,
		b:      math.MaxInt64,
		expect: math.MaxInt64,
	}, {
		a:      math.MaxInt64,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
		a:      math.MinInt64,
		b:      math.MaxInt64,
		expect: math.MinInt64,
	}, {
		a:      math.MinInt64,
		b:      math.MinInt64,
		expect: math.MaxInt64,
	}, {
		a:      100,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
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
