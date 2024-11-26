package starlark_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestOldSafeAdd(t *testing.T) {
	tests := []struct {
		name         string
		a, b, expect int
	}{{
		name:   "0+0",
		a:      0,
		b:      0,
		expect: 0,
	}, {
		name:   "-100+-100",
		a:      -100,
		b:      -100,
		expect: -200,
	}, {
		name:   "0x80...01+-10",
		a:      math.MinInt + 1,
		b:      -10,
		expect: math.MinInt,
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
			if actual := starlark.OldSafeAdd(test.a, test.b); actual != test.expect {
				t.Errorf("incorrect result: expected %d but got %d", test.expect, actual)
			}
		})
	}
}

func TestOldSafeAdd64(t *testing.T) {
	tests := []struct {
		name         string
		a, b, expect int64
	}{{
		name:   "9+16",
		a:      9,
		b:      16,
		expect: 25,
	}, {
		name:   "-100+-100",
		a:      -100,
		b:      -100,
		expect: -200,
	}, {
		name:   "0x80...01+-10",
		a:      math.MinInt64 + 1,
		b:      -10,
		expect: math.MinInt64,
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
			if actual := starlark.OldSafeAdd64(test.a, test.b); actual != test.expect {
				t.Errorf("incorrect result: expected %d but got %d", test.expect, actual)
			}
		})
	}
}

func TestOldSafeMul(t *testing.T) {
	// This value has been identified by klee to be representative of many codepaths.
	var limitIdentityOrNegator int
	const intSize = 32 << (^uint(0) >> 63) // 32 or 64
	switch intSize {
	case 32:
		limitIdentityOrNegator = -2012741632
	case 64:
		limitIdentityOrNegator = -1 // TODO(kcza): get the magic constant here.
	default:
		t.Fatal("unsupported int width")
	}

	tests := []struct {
		name         string
		a, b, expect int
	}{{
		name:   "0*2",
		a:      0,
		b:      2,
		expect: 0,
	}, {
		name:   "2*0",
		a:      2,
		b:      0,
		expect: 0,
	}, {
		name:   "2*2",
		a:      2,
		b:      2,
		expect: 4,
	}, {
		name:   "MinInt*2",
		a:      math.MinInt,
		b:      2,
		expect: math.MinInt,
	}, {
		name:   "MaxInt*2",
		a:      math.MaxInt,
		b:      2,
		expect: math.MaxInt,
	}, {
		name:   "MinInt*MaxInt",
		a:      math.MinInt,
		b:      math.MaxInt,
		expect: math.MinInt,
	}, {
		name:   "MaxInt*MinInt",
		a:      math.MaxInt,
		b:      math.MinInt,
		expect: math.MinInt,
	}, {
		name:   "a*b=0,a!=0,b!=0",
		a:      limitIdentityOrNegator,
		b:      math.MinInt,
		expect: math.MaxInt,
	}, {
		name:   "a*b=-a,a!=0,b!=-1",
		a:      limitIdentityOrNegator,
		b:      math.MaxInt,
		expect: math.MinInt,
	}, {
		name:   "a*b=b,a!=1",
		a:      1<<10 + 1,
		b:      math.MinInt / 2,
		expect: math.MinInt,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := starlark.OldSafeMul(test.a, test.b); actual != test.expect {
				t.Errorf("incorrect result: expected %d but got %d", test.expect, actual)
			}
		})
	}
}

func TestOldSafeMul64(t *testing.T) {
	const limitIdentityOrNegator = -1 // TODO(kcza): get the magic constant.
	tests := []struct {
		name         string
		a, b, expect int64
	}{{
		name:   "0*2",
		a:      0,
		b:      2,
		expect: 0,
	}, {
		name:   "2*0",
		a:      2,
		b:      0,
		expect: 0,
	}, {
		name:   "2*2",
		a:      2,
		b:      2,
		expect: 4,
	}, {
		name:   "MinInt*2",
		a:      math.MinInt64,
		b:      2,
		expect: math.MinInt64,
	}, {
		name:   "MaxInt*2",
		a:      math.MaxInt64,
		b:      2,
		expect: math.MaxInt64,
	}, {
		name:   "MinInt*MaxInt",
		a:      math.MinInt64,
		b:      math.MaxInt64,
		expect: math.MinInt64,
	}, {
		name:   "MaxInt*MinInt",
		a:      math.MaxInt64,
		b:      math.MinInt64,
		expect: math.MinInt64,
	}, {
		name:   "a*b=0,a!=0,b!=0",
		a:      limitIdentityOrNegator,
		b:      math.MinInt64,
		expect: math.MaxInt64,
	}, {
		name:   "a*b=-a,a!=0,b!=-1",
		a:      limitIdentityOrNegator,
		b:      math.MaxInt64,
		expect: math.MinInt64,
	}, {
		name:   "a*b=b,a!=1",
		a:      1<<10 + 1,
		b:      math.MinInt64 / 2,
		expect: math.MinInt64,
	}}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d + %d", test.a, test.b), func(t *testing.T) {
			if actual := starlark.OldSafeMul64(test.a, test.b); actual != test.expect {
				t.Errorf("incorrect result: expected %d but got %d", test.expect, actual)
			}
		})
	}
}
