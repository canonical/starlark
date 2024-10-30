package starlark_test

import (
	"math"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestValidSafeintRoundtrip(t *testing.T) {
	runRoundtripTest(t, "int", starlark.SafeInteger.Int, 100, math.MinInt)
	runRoundtripTest(t, "int64", starlark.SafeInteger.Int64, 100, math.MinInt64)
	runRoundtripTest(t, "int32", starlark.SafeInteger.Int32, 100, math.MaxInt32+1)
	runRoundtripTest(t, "int16", starlark.SafeInteger.Int16, 100, math.MaxInt32)
	runRoundtripTest(t, "int8", starlark.SafeInteger.Int8, 100, math.MaxInt32)

	runRoundtripTest(t, "uint", starlark.SafeInteger.Uint, 100, -1)
	runRoundtripTest(t, "uint64", starlark.SafeInteger.Uint64, 100, -1)
	runRoundtripTest(t, "uint32", starlark.SafeInteger.Uint32, 100, -1)
	runRoundtripTest(t, "uint16", starlark.SafeInteger.Uint16, 100, -1)
	runRoundtripTest(t, "uint8", starlark.SafeInteger.Uint8, 100, -1)
}

// FIXME al posto di shouldFail, passa qualcosa che fallisce
func runRoundtripTest[Int starlark.Integer](t *testing.T, name string, extract func(starlark.SafeInteger) (Int, bool), pass Int, fail int64) {
	t.Run(name, func(t *testing.T) {
		t.Run("pass", func(t *testing.T) {
			si := starlark.SafeInt(pass)
			back, ok := extract(si)
			if !ok {

				t.Errorf("expected success, got error")
			}
			if back != pass {
				t.Errorf("roundtrip mismatch: want %d, got %d", pass, back)
			}
		})

		t.Run("fails", func(t *testing.T) {
			si := starlark.SafeInt(fail)
			back, ok := extract(si)
			if ok {
				t.Errorf("expected error, got %d", back)
			}
		})
	})
}
