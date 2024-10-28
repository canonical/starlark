package starlark_test

import (
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestValidSafeintRoundtrip(t *testing.T) {
	tests := []struct{
		name string
		value interface{}
	}{{
		name: "int64",
		value: 100,
	}, {
		name: "uint64",
		value: 100,
	}}
	for _, test := range tests {
		safeInt := starlark.SafeInt(test.value)

		if !ok {
			t.Errorf("expected ok")
		} else if value != smallInt {
			t.Errorf("incorrect value")
		}
	}
	smallInt :=

	i64, ok := starlark.SafeInt(smallIntValue).Int64()
	checkIs(i64, smallIntValue, ok)

	u64, ok := starlark.SafeInt

	const invalidInt = 10.0
}
