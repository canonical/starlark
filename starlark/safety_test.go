package starlark_test

import (
	"fmt"
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
}

func testSafetyFlags(t *testing.T, require, probe starlark.SafetyFlags, expectPass bool) {
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
	const invalidFlags = starlark.SafetyFlags(0xdebac1e)

	if err := validFlags.MustBeValid(); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := invalidFlags.MustBeValid(); err == nil {
		t.Errorf("No error when checking invalid flags")
	} else if !strings.HasPrefix(err.Error(), "Invalid safety flags: ") {
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

func TestSafetyFlagNamesAreUnique(t *testing.T) {
	const nonIdentSep = "@"

	knownFlags := make(map[string]struct{}, 1+starlark.Safe)
	for f := starlark.NotSafe; f <= starlark.Safe; f++ {
		key := strings.Join(f.Names(), nonIdentSep)
		if _, ok := knownFlags[key]; ok {
			t.Errorf("Duplicate names set for flags %v", f)
		}
		knownFlags[key] = struct{}{}
	}
}

func TestSafetyFlagsAreStoredAgainstFunctions(t *testing.T) {
	const expectedSafety = starlark.MemSafe | starlark.IOSafe

	f := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("f"), nil
	}

	if err := starlark.DeclareBuiltinFuncSafety(f, expectedSafety); err != nil {
		t.Errorf("Failed to declare valid safety: %v", err)
	}
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

	const commonSafety = starlark.MemSafe
	const closure1DefinedSafety = commonSafety | starlark.CPUSafe
	const closure2DefinedSafety = commonSafety | starlark.IOSafe

	closure1 := base("foo")
	closure2 := base("bar")
	if err := starlark.DeclareBuiltinFuncSafety(closure1, closure1DefinedSafety); err != nil {
		t.Errorf("Failed to declare valid safety: %v", err)
	}
	if err := starlark.DeclareBuiltinFuncSafety(closure2, closure2DefinedSafety); err != nil {
		t.Errorf("Failed to declare valid safety: %v", err)
	}

	testSafetyBounded := func(least, most starlark.SafetyFlags, closure func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error)) {
		closureSafety := starlark.SafetyOfBuiltinFunc(closure)
		if closureSafety != least && closureSafety != most {
			fnPtr := reflect.ValueOf(closure).Pointer()
			t.Errorf("Closure @ %#x had incorrect safety flags: expected either %v or %v but got %v", fnPtr, least, most, closureSafety)
		}
	}

	// Ensure that the closures have at least a set of safety flags, with no extras defined
	testSafetyBounded(commonSafety, closure1DefinedSafety, closure1)
	testSafetyBounded(commonSafety, closure2DefinedSafety, closure2)
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
	const expectedSafety = starlark.CPUSafe | starlark.TimeSafe
	b := starlark.NewBuiltin("func", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	if err := b.DeclareSafety(expectedSafety); err != nil {
		t.Errorf("Unexpected error declaring safety: %v", err)
	}
	if safety := b.Safety(); safety != expectedSafety {
		t.Errorf("Incorrect safety reported, expected %v but got %v", expectedSafety, safety)
	}
}

func TestBuiltinFuncSafety(t *testing.T) {
	const expectedSafety = starlark.IOSafe | starlark.MemSafe

	fn := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	}
	if err := starlark.DeclareBuiltinFuncSafety(fn, expectedSafety); err != nil {
		t.Errorf("Failed to declare valid safety: %v", err)
	}

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
	if b, err := starlark.NewBuiltinWithSafety("fn", fn, expectedSafety); err != nil {
		t.Errorf("Unexpected error declaring new safe builtin: %v", err)
	} else if safety := b.Safety(); safety != expectedSafety {
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
	if err := starlark.DeclareSafety(instance1, expectedSafety); err != nil {
		t.Errorf("Failed to declare safety: %v", err)
	}
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
	if err := starlark.DeclareBuiltinFuncSafety(fn, fnSafety); err != nil {
		t.Errorf("Failed to declare valid safety: %v", err)
	}

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
	if err := starlark.DeclareBuiltinFuncSafety(fn, fnSafety); err != nil {
		t.Errorf("Failed to declare valid safety: %v", err)
	}

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
	if err := starlark.DeclareSafety(new(SafeCallable), starlark.Safe); err != nil {
		panic(fmt.Sprintf("Failed to declare safety: %v", err))
	}
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
		if e := err.Error(); !strings.HasPrefix(e, "missing safety flags: ") {
			t.Errorf("Unexpected error when calling unsafe callable: %v", e)
		}
	} else {
		t.Errorf("Expected unsafe callable to be rejected by thread with defined safety")
	}
}
