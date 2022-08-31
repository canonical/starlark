package starlark_test

import (
	"fmt"
	"math/bits"
	"reflect"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestSafety(t *testing.T) {
	justCpu := starlark.CPUSafe
	justMem := starlark.MemSafe
	memAndCpu := justCpu | justMem
	unrestricted := starlark.NotSafe

	if err := unrestricted.MustPermit(memAndCpu); err != nil {
		t.Errorf("Incorrect safety failure %v", err)
	}

	if err := justCpu.MustPermit(memAndCpu); err != nil {
		t.Errorf("Incorrect safety failure: %v", err)
	}

	if memAndCpu.MustPermit(justCpu) == nil {
		t.Errorf("Safety flags did not apply: missing flag not rejected")
	}

	if memAndCpu.MustPermit(unrestricted) == nil {
		t.Errorf("Failed to enforce safety: restricted env allows unrestricted")
	}
}

func TestSafetyFlags(t *testing.T) {
	// Equal safety-sets are accepted
	testSafetyFlags(t, starlark.NotSafe, starlark.NotSafe, true)
	testSafetyFlags(t, starlark.Safe, starlark.Safe, true)

	testSafetyFlags(t, starlark.NotSafe, starlark.Safe, true)  // Where no safety is expected, something with stronger safety is permitted
	testSafetyFlags(t, starlark.Safe, starlark.NotSafe, false) // Where full safety is expected, no-safety is rejected

	// Disjoint non-empty safety sets are rejected
	const disjointA = starlark.TimeSafe | starlark.IOSafe
	const disjointB = starlark.MemSafe | starlark.CPUSafe
	testSafetyFlags(t, disjointA, disjointB, false)
	testSafetyFlags(t, disjointB, disjointA, false)

	// Symmetrically-different safety sets are rejected
	const common = starlark.TimeSafe | starlark.IOSafe
	const symmetricallyDifferentA = starlark.MemSafe | common
	const symmetricallyDifferentB = starlark.CPUSafe | common
	testSafetyFlags(t, symmetricallyDifferentA, symmetricallyDifferentB, false)
	testSafetyFlags(t, symmetricallyDifferentB, symmetricallyDifferentA, false)

	// A superset of required safety is accepted
	testSafetyFlags(t, common, symmetricallyDifferentA, true)

	// Invalid flags rejected
	const valid = starlark.Safe
	const invalid = starlark.Safety(0xbadc0de)
	testSafetyFlags(t, valid, invalid, false)
}

func testSafetyFlags(t *testing.T, require, probe starlark.Safety, expectPass bool) {
	if actual := require.Permits(probe); actual != expectPass {
		t.Errorf("Safety flag checking did not return correct value: expected %v but got %v", expectPass, actual)
	}

	if err := require.MustPermit(probe); expectPass && err != nil {
		t.Errorf("Safety flag checking returned unexpected error: checking that %v permits %v returned %v", require, probe, err)
	} else if !expectPass && err == nil {
		t.Errorf("Safety flag checking did not return an error when expected")
	}
}

func TestSafetyFlagChecking(t *testing.T) {
	const validFlags = starlark.MemSafe
	const invalidFlags = starlark.Safety(0xdebac1e)

	if err := validFlags.CheckValid(); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := invalidFlags.CheckValid(); err == nil {
		t.Errorf("No error when checking invalid flags")
	} else if !strings.HasSuffix(err.Error(), "invalid safety flags") {
		t.Errorf("Unexpected error when checking invalid flags: %v", err)
	}
}

func TestDefaultStoredSafetyIsZero(t *testing.T) {
	b := starlark.NewBuiltin("func", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	if storedSafety := b.Safety(); storedSafety != 0 {
		t.Errorf("Default safety is not zero: got %d", storedSafety)
	}
}

func TestSafetyFlagNameOrder(t *testing.T) {
	tests := map[starlark.Safety]string{
		starlark.NotSafe:  "NotSafe",
		starlark.CPUSafe:  "CPUSafe",
		starlark.MemSafe:  "MemSafe",
		starlark.TimeSafe: "TimeSafe",
		starlark.IOSafe:   "IOSafe",
		starlark.Safe:     "(CPUSafe|MemSafe|TimeSafe|IOSafe)",
	}

	flagWidth := reflect.TypeOf(starlark.NotSafe).Size() * (bits.UintSize / reflect.TypeOf(uint(0)).Size())
	for i := *starlark.NumSafetyFlagBitsDefined; i < uint(flagWidth); i++ {
		flag := starlark.Safety(1 << i)
		tests[flag] = fmt.Sprintf("InvalidSafe(%d)", flag)
	}

	for safety, expected := range tests {
		if actual := safety.String(); actual != expected {
			t.Errorf("Expected %s but got %s", expected, actual)
		}
	}
}

func TestBuiltinClosuresInteractSafely(t *testing.T) {
	base := func(s string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(s), nil
		}
	}

	const expectedClosure1Safety = starlark.MemSafe | starlark.CPUSafe
	const expectedClosure2Safety = starlark.MemSafe | starlark.IOSafe

	builtinClosure1, err := starlark.NewBuiltinWithSafety("foo", expectedClosure1Safety, base("foo"))
	if err != nil {
		t.Errorf("Unexpected error defining closure with valid safety flags: %v", err)
	}
	builtinClosure2, err := starlark.NewBuiltinWithSafety("bar", expectedClosure2Safety, base("bar"))
	if err != nil {
		t.Errorf("Unexpected error defining closure with valid safety flags: %v", err)
	}

	if closure1Safety := builtinClosure1.Safety(); closure1Safety != expectedClosure1Safety {
		t.Errorf("Incorrect safety reported on closure: expected %s but got %s", expectedClosure1Safety, closure1Safety)
	}
	if closure2Safety := builtinClosure2.Safety(); closure2Safety != expectedClosure2Safety {
		t.Errorf("Incorrect safety reported on closure: expected %s but got %s", expectedClosure2Safety, closure2Safety)
	}
}

func TestFunctionSafeExecution(t *testing.T) {
	// Ensure that starlark-defined functions can always be run
	const prog = `
def func():
	pass
func()
`
	thread := new(starlark.Thread)
	thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)

	_, err := starlark.ExecFile(thread, "func_safety_test.go", prog, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestLambdaSafeExecution(t *testing.T) {
	// Ensure that lambdas can always be run
	const prog = `(lambda x: x)(1)`
	thread := new(starlark.Thread)
	thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)

	_, err := starlark.ExecFile(thread, "lambda_safety_test.go", prog, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestBuiltinSafeExecution(t *testing.T) {
	thread := new(starlark.Thread)
	thread.RequireSafety(starlark.CPUSafe | starlark.TimeSafe)

	t.Run("Builtin=Permitted", func(t *testing.T) {
		const permittedSafety = starlark.Safe

		fn := starlark.NewBuiltin("fn", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("foo"), nil
		})
		if err := fn.DeclareSafety(permittedSafety); err != nil {
			t.Errorf("Unexpected error declaring valid safety: %v", err)
		}
		env := starlark.StringDict{"fn": fn}

		if _, err := starlark.ExecFile(thread, "builtin_safety_restrictions", "fn()", env); err != nil {
			t.Errorf("Unexpected error executing safe builtin: %v", err)
		}
	})

	t.Run("Builtin=Forbidden", func(t *testing.T) {
		const forbiddenSafety = starlark.NotSafe

		fn := starlark.NewBuiltin("fn", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("foo"), nil
		})
		if err := fn.DeclareSafety(forbiddenSafety); err != nil {
			t.Errorf("Unexpected error declaring valid safety: %v", err)
		}
		env := starlark.StringDict{"fn": fn}

		if _, err := starlark.ExecFile(thread, "builtin_safety_restrictions", "fn()", env); err == nil {
			t.Errorf("Expected error when trying to execute forbidden builtin")
		} else if err.Error() != "feature unavailable to the sandbox" {
			t.Errorf("Unexpected error executing safe builtin: %v", err)
		}
	})
}

type dummyCallable struct{ safety starlark.Safety }

var (
	_ starlark.Value       = &dummyCallable{}
	_ starlark.Callable    = &dummyCallable{}
	_ starlark.SafetyAware = &dummyCallable{}
)

func (dummyCallable) String() string        { return "" }
func (dummyCallable) Type() string          { return "dummyCallable" }
func (dummyCallable) Freeze()               {}
func (dummyCallable) Truth() starlark.Bool  { return false }
func (dummyCallable) Hash() (uint32, error) { return 0, nil }
func (dummyCallable) Name() string          { return "dummyCallable" }
func (dummyCallable) CallInternal(*starlark.Thread, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
}
func (d *dummyCallable) Safety() starlark.Safety             { return d.safety }
func (d *dummyCallable) DeclareSafety(flags starlark.Safety) { d.safety = flags }

func TestCallableSafeExecution(t *testing.T) {
	thread := new(starlark.Thread)
	thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
	c := &dummyCallable{}
	c.DeclareSafety(starlark.MemSafe)
	const prog = "c()"
	env := starlark.StringDict{
		"c": c,
	}

	// Permit
	c.DeclareSafety(starlark.Safe)
	if _, err := starlark.ExecFile(thread, "dynamic_safety_checking", prog, env); err != nil {
		t.Errorf("Unexpected error running permitted function %v", err)
	}

	// Forbid
	c.DeclareSafety(starlark.NotSafe)
	if _, err := starlark.ExecFile(thread, "dynamic_safety_checking", prog, env); err == nil {
		t.Errorf("Expected error running dynamically-forbidden function")
	} else if err.Error() != "feature unavailable to the sandbox" {
		t.Errorf("Unexpected error running forbidden function %v", err)
	}

	// Repermit
	c.DeclareSafety(starlark.Safe)
	if _, err := starlark.ExecFile(thread, "dynamic_safety_checking", prog, env); err != nil {
		t.Errorf("Unexpected error running dynamically re-permitted function %v", err)
	}
}

func TestNewBuiltinWithSafety(t *testing.T) {
	const expectedSafety = starlark.IOSafe | starlark.MemSafe
	fn := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	}
	if b, err := starlark.NewBuiltinWithSafety("fn", expectedSafety, fn); err != nil {
		t.Errorf("Unexpected error declaring new safe builtin: %v", err)
	} else if safety := b.Safety(); safety != expectedSafety {
		t.Errorf("Incorrect stored safety flags: expected %v but got %v", expectedSafety, safety)
	}
}
