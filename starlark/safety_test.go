package starlark_test

import (
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestSafety(t *testing.T) {
	justCpu := starlark.CPUSafe
	justMem := starlark.MemSafe
	memAndCpu := justCpu | justMem
	unrestricted := starlark.SafetyFlags(0)

	if err := unrestricted.Permits(memAndCpu); err != nil {
		t.Errorf("Incorrect safety failure %v", err)
	}

	if err := justCpu.Permits(memAndCpu); err != nil {
		t.Errorf("Incorrect safety failure: %v", err)
	}

	if memAndCpu.Permits(justCpu) == nil {
		t.Errorf("Safety flags did not apply: missing flag not rejected")
	}

	if memAndCpu.Permits(unrestricted) == nil {
		t.Errorf("Failed to enforce safety: restricted env allows unrestricted")
	}
}

func TestSafetyFlags(t *testing.T) {
	const noSafety = starlark.SafetyFlags(0)
	const fullSafety = starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe

	// Equal safety-sets are accepted
	testSafetyFlags(t, noSafety, noSafety, true)
	testSafetyFlags(t, fullSafety, fullSafety, true)

	testSafetyFlags(t, noSafety, fullSafety, true)  // Where no safety is expected, something with stronger safety is permitted
	testSafetyFlags(t, fullSafety, noSafety, false) // Where full safety is expected, no-safety is rejected

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
}

func testSafetyFlags(t *testing.T, require, probe starlark.SafetyFlags, expectPass bool) {
	err := require.Permits(probe)
	if expectPass && err != nil {
		t.Errorf("Safety flag checking returned unexpected error: checking that %v permits %v returned %v", require, probe, err)
	} else if !expectPass && err == nil {
		t.Errorf("Safety flag checking did not return an error when expected")
	}
}

func TestInvalidSafetyFlagsRejected(t *testing.T) {
	const invalidFlags = starlark.SafetyFlags(0xdebac1e)
	const validFlags = starlark.MemSafe
	expected := "Invalid safety flags: got 0xdebac1e"

	defer func() {
		if r := recover(); r != nil {
			if r != expected {
				t.Errorf("Expected invalid safety flags to be reported: expected message '%s' but got different panic '%s'", expected, r)
			}
		} else {
			t.Error("AssertValid did not panic on invalid flags")
		}
	}()

	validFlags.AssertValid()   // Should not panic
	invalidFlags.AssertValid() // Should panic
}

func TestDefaultStoredSafetyIsZero(t *testing.T) {
	b := starlark.NewBuiltin("func", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	if storedSafety := b.Safety(); storedSafety != 0 {
		t.Errorf("Default safety is not zero: got %d", storedSafety)
	}
}

func TestSafetyFlagsAreStoredAgainstFunctions(t *testing.T) {
	const expectedSafety = starlark.MemSafe | starlark.IOSafe

	f := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("f"), nil
	}

	starlark.DeclareBuiltinFuncSafety(f, expectedSafety)
	if actualSafety := starlark.SafetyOfBuiltinFunc(f); actualSafety != expectedSafety {
		t.Errorf("Incorrect safety flags, expected %v but got %v", expectedSafety, actualSafety)
	}
}

func TestClosuresInteractSafely(t *testing.T) {
	// base :: string -> () -> string
	base := func(s string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(s), nil
		}
	}

	const expectedClosure1Safety = starlark.MemSafe | starlark.CPUSafe
	const expectedClosure2Safety = starlark.MemSafe | starlark.IOSafe

	closure1 := base("foo")
	closure2 := base("bar")
	starlark.DeclareBuiltinFuncSafety(closure1, expectedClosure1Safety)
	starlark.DeclareBuiltinFuncSafety(closure2, expectedClosure2Safety)

	if closure1Safety := starlark.SafetyOfBuiltinFunc(closure1); closure1Safety != expectedClosure1Safety {
		t.Errorf("Closure had incorrect safety, expected %v but got %v", expectedClosure1Safety, closure1Safety)
	}
	if closure2Safety := starlark.SafetyOfBuiltinFunc(closure2); closure2Safety != expectedClosure2Safety {
		t.Errorf("Closure had incorrect safety, expected %v but got %v", expectedClosure2Safety, closure2Safety)
	}
}

func TestFunctionSafety(t *testing.T) {
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

func TestLambdaSafety(t *testing.T) {
	// Ensure that lambdas can always be run
	const prog = `(lambda x: x)(1)`
	thread := new(starlark.Thread)
	thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)

	_, err := starlark.ExecFile(thread, "lambda_safety_test.go", prog, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestBuiltinSafety(t *testing.T) {
	const expectedSafety = starlark.IOSafe | starlark.MemSafe

	fn := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	}
	starlark.DeclareBuiltinFuncSafety(fn, expectedSafety)

	instance1 := starlark.NewBuiltin("b1", fn)
	instance2 := starlark.NewBuiltin("b2", fn)

	if instance1Safety := starlark.SafetyOfBuiltinFunc(fn); instance1Safety != expectedSafety {
		t.Errorf("Builtin's underlying function had incorrect safety: expected %d but got %d", expectedSafety, instance1Safety)
	}
	if instance1Safety := instance1.Safety(); instance1Safety != expectedSafety {
		t.Errorf("Builtin instance had incorrect safety: expected %d but got %d", expectedSafety, instance1Safety)
	}
	if instance2Safety := instance2.Safety(); instance2Safety != expectedSafety {
		t.Errorf("Builtin instance did not share the safety of the function which underlies it: expected %d but got %d", expectedSafety, instance2Safety)
	}
}

func TestNewBuiltinWithSafety(t *testing.T) {
	const expectedSafety = starlark.IOSafe | starlark.MemSafe
	fn := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	}
	b := starlark.NewBuiltinWithSafety("fn", fn, expectedSafety)
	if safety := b.Safety(); safety != expectedSafety {
		t.Errorf("Incorrect stored safety flags: expected %v but got %v", expectedSafety, safety)
	}
}

type dummyCallable struct{ string }

var _ starlark.Callable = &dummyCallable{}

func (dummyCallable) String() string        { return "" }
func (dummyCallable) Type() string          { return "dummyCallable" }
func (dummyCallable) Freeze()               {}
func (dummyCallable) Truth() starlark.Bool  { return false }
func (dummyCallable) Hash() (uint32, error) { return 0, nil }
func (dummyCallable) Name() string          { return "dummyCallable" }
func (dummyCallable) CallInternal(*starlark.Thread, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
}

func TestCallableSafety(t *testing.T) {
	const expectedSafety = starlark.MemSafe | starlark.TimeSafe

	instance1 := dummyCallable{}
	starlark.DeclareSafety(instance1, expectedSafety)
	instance2 := dummyCallable{}

	if instance1Safety := starlark.SafetyOf(instance1); instance1Safety != expectedSafety {
		t.Errorf("Failed to declare safety on callable: expected flags %v but got %v", expectedSafety, instance1Safety)
	}
	if instance2Safety := starlark.SafetyOf(instance2); instance2Safety != expectedSafety {
		t.Errorf("Safety was not shared between instances of callable: expected flags %v but got %v", expectedSafety, instance2Safety)
	}
}

func TestSafeBuiltinCanExecuteSafety(t *testing.T) {
	const requiredSafety = starlark.CPUSafe
	const fnSafety = requiredSafety | starlark.MemSafe
	const prog = "fn()"

	thread := new(starlark.Thread)
	thread.RequireSafety(requiredSafety)

	fn := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	}
	starlark.DeclareBuiltinFuncSafety(fn, fnSafety)

	env := starlark.StringDict{
		"fn": starlark.NewBuiltin("fn", fn),
	}

	if _, err := starlark.ExecFile(thread, "test_permitted_safety", prog, env); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestUnsafeBuiltinCannotExecuteSafety(t *testing.T) {
	const requiredSafety = starlark.CPUSafe
	const fnSafety = 0
	const prog = "fn()"

	thread := new(starlark.Thread)
	thread.RequireSafety(requiredSafety)

	fn := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	}
	starlark.DeclareBuiltinFuncSafety(fn, fnSafety)

	env := starlark.StringDict{
		"fn": starlark.NewBuiltin("fn", fn),
	}

	if _, err := starlark.ExecFile(thread, "test_unpermitted_safety", prog, env); err == nil {
		t.Errorf("Expected error but got none")
	}
}

type SafeCallable struct{ string }

var _ starlark.Callable = &SafeCallable{}

func (sc SafeCallable) String() string     { return sc.string }
func (SafeCallable) Type() string          { return "SafeCallable" }
func (SafeCallable) Freeze()               {}
func (SafeCallable) Truth() starlark.Bool  { return false }
func (SafeCallable) Hash() (uint32, error) { return 0, nil }
func (SafeCallable) Name() string          { return "SafeCallable" }
func (SafeCallable) CallInternal(*starlark.Thread, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
}
func init() {
	const strongSafety = starlark.CPUSafe | starlark.IOSafe | starlark.MemSafe | starlark.TimeSafe
	starlark.DeclareSafety(new(SafeCallable), strongSafety)
}

func TestSafeCallableCanExecute(t *testing.T) {
	const requiredSafety = starlark.CPUSafe

	thread := new(starlark.Thread)
	thread.RequireSafety(requiredSafety)

	const prog = "c()"
	env := starlark.StringDict{
		"c": new(SafeCallable),
	}

	if _, err := starlark.ExecFile(thread, "test_safe_callable_permitted", prog, env); err != nil {
		t.Errorf("Unexpected error while running safe callable: %v", err)
	}
}

type UnsafeCallable struct{}

var _ starlark.Callable = &SafeCallable{}

func (UnsafeCallable) String() string        { return "" }
func (UnsafeCallable) Type() string          { return "UnsafeCallable" }
func (UnsafeCallable) Freeze()               {}
func (UnsafeCallable) Truth() starlark.Bool  { return false }
func (UnsafeCallable) Hash() (uint32, error) { return 0, nil }
func (UnsafeCallable) Name() string          { return "UnsafeCallable" }
func (UnsafeCallable) CallInternal(*starlark.Thread, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
}

func TestUnsafeCallableCannotExecute(t *testing.T) {
	const requiredSafety = starlark.CPUSafe

	thread := new(starlark.Thread)
	thread.RequireSafety(requiredSafety)

	const prog = "c()"
	env := starlark.StringDict{
		"c": new(UnsafeCallable),
	}

	if _, err := starlark.ExecFile(thread, "test_unsafe_callable_rejected", prog, env); err != nil {
		if e := err.Error(); !strings.HasPrefix(e, "Missing safety flags: ") {
			t.Errorf("Unexpected error when calling unsafe callable: %v", e)
		}
	} else {
		t.Errorf("Expected unsafe callable to be rejected by thread with defined safety")
	}
}
