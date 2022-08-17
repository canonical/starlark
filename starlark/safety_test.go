package starlark_test

import (
	"fmt"
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

func TestInvalidSafetyRejected(t *testing.T) {
	const invalidFlags = starlark.SafetyFlags(0xdebac1e)
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

	// Should panic
	invalidFlags.AssertValid()
}

func TestSafetyFlagsAreStoredAgainstFunctions(t *testing.T) {
	const safety = starlark.MemSafe | starlark.IOSafe

	f := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("f"), nil
	}

	starlark.DeclareBuiltinFuncSafety(f, safety)
	if actualSafety := starlark.SafetyOfBuiltinFunc(f); actualSafety != safety {
		t.Errorf("Incorrect safety flags, expected %v but got %v", safety, actualSafety)
	}

	bf1 := starlark.NewBuiltin("b1", f)
	bf2 := starlark.NewBuiltin("b2", f)

	if b1Safety := bf1.Safety(); b1Safety != safety {
		t.Errorf("Incorrect safety for builtin: expected %v but got %v", safety, b1Safety)
	}
	if b2Safety := bf2.Safety(); b2Safety != safety {
		t.Errorf("Incorrect safety for builtin: expected %v but got %v", safety, b2Safety)
	}

	g := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("g"), nil
	}
	bg := starlark.NewBuiltin("bg", g)
	bg.DeclareSafety(safety)
	if gSafety := starlark.SafetyOfBuiltinFunc(g); gSafety != safety {
		t.Errorf("Safety was not declared against the underlying function: expected %v but got %v", safety, gSafety)
	}
}

type dummyCallable struct{ string }

var _ starlark.Callable = &dummyCallable{}

func (dummyCallable) String() string        { return "" }
func (dummyCallable) Type() string          { return "dummyCallable" }
func (dummyCallable) Freeze()               {}
func (dummyCallable) Truth() starlark.Bool  { return false }
func (dummyCallable) Hash() (uint32, error) { return 0, nil }
func (dc dummyCallable) Name() string       { return "dummyCallable" }
func (dc dummyCallable) CallInternal(*starlark.Thread, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
}

func TestCallableSafety(t *testing.T) {
	const safety = starlark.MemSafe | starlark.TimeSafe

	instance1 := dummyCallable{"foo"}
	starlark.DeclareCallableFuncSafety(instance1.CallInternal, safety)
	instance2 := dummyCallable{"bar"}

	if instance1Safety := starlark.SafetyOfCallableFunc(instance1.CallInternal); instance1Safety != safety {
		t.Errorf("Failed to declare safety on callable: expected flags %v but got %v", safety, instance1Safety)
	}
	if instance2Safety := starlark.SafetyOfCallableFunc(instance2.CallInternal); instance2Safety != safety {
		t.Errorf("Safety was not shared between instances of callable: expected flags %v but got %v", safety, instance2Safety)
	}
}

type uniqueBuiltinFunctions []func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error)

func (s uniqueBuiltinFunctions) Iter() func() func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	i := -1
	return func() func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		i++
		if i >= len(s) {
			panic(fmt.Sprintf("Too few unique builtin test functions were declared: only %d available", len(s)))
		}
		return s[i]
	}
}

func TestSafetyEnforcement(t *testing.T) {
	// As safety is tied to the underlying function, multiple unique functions are required in isolation
	builtinFuncs := uniqueBuiltinFunctions{
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
		func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.String("Hello, world!"), nil
		},
	}
	getBuiltin := builtinFuncs.Iter()

	const noSafety = starlark.SafetyFlags(0)
	const fullSafety = starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe

	// Equal safety-sets are accepted
	testSafetyEnforcement(t, noSafety, noSafety, true, getBuiltin())
	testSafetyEnforcement(t, fullSafety, fullSafety, true, getBuiltin())

	testSafetyEnforcement(t, noSafety, fullSafety, true, getBuiltin())  // Where no safety is expected, something with stronger safety is permitted
	testSafetyEnforcement(t, fullSafety, noSafety, false, getBuiltin()) // Where full safety is expected, no-safety is rejected

	// Disjoint non-empty safety sets are rejected
	const disjointA = starlark.TimeSafe | starlark.IOSafe
	const disjointB = starlark.MemSafe | starlark.CPUSafe
	testSafetyEnforcement(t, disjointA, disjointB, false, getBuiltin())
	testSafetyEnforcement(t, disjointB, disjointA, false, getBuiltin())

	// Symmetrically-different safety sets are rejected
	const common = starlark.TimeSafe | starlark.IOSafe
	const symmetricallyDifferentA = starlark.MemSafe | common
	const symmetricallyDifferentB = starlark.CPUSafe | common
	testSafetyEnforcement(t, symmetricallyDifferentA, symmetricallyDifferentB, false, getBuiltin())
	testSafetyEnforcement(t, symmetricallyDifferentB, symmetricallyDifferentA, false, getBuiltin())

	// A superset of required safety is accepted
	testSafetyEnforcement(t, common, symmetricallyDifferentA, true, getBuiltin())
}

func testSafetyEnforcement(t *testing.T, require, probe starlark.SafetyFlags, expectPass bool, fn func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error)) {
	thread := new(starlark.Thread)
	thread.RequireSafety(require)

	const prog = `func()`
	b := starlark.NewBuiltin("func", fn)
	b.DeclareSafety(probe)
	predecls := starlark.StringDict{
		"func": b,
	}
	_, err := starlark.ExecFile(thread, "safety_test.star", prog, predecls)
	if expectPass && err != nil {
		t.Errorf("Unexpected cancellation when testing safety (require: %x, probe: %x, th: %x, fn: %x): %v", require, probe, thread.Safety(), b.Safety(), err)
	} else if !expectPass && err == nil {
		t.Errorf("Safety enforcement did not error when expected")
	}
}

func TestThreadSafetySetOnlyGrows(t *testing.T) {
	const initialFlags = starlark.CPUSafe | starlark.MemSafe
	const newFlags = starlark.IOSafe | starlark.TimeSafe
	const expectedFlags = initialFlags | newFlags

	thread := new(starlark.Thread)
	thread.RequireSafety(initialFlags)

	if thread.Safety() != initialFlags {
		t.Errorf("Safety flags differ from declaration: expected %v but got %v", initialFlags.Names(), thread.Safety().Names())
	}

	thread.RequireSafety(newFlags)

	if thread.Safety() != expectedFlags {
		missing := thread.Safety() &^ expectedFlags
		t.Errorf("Missing safety flags %v, expected %v", missing.Names(), expectedFlags.Names())
	}
}

func TestStarlarkDefinedFunctionSafetyIsPermissive(t *testing.T) {
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

func TestLambdaSafetyIsPermissive(t *testing.T) {
	// Ensure that lambdas can always be run
	const prog = `(lambda x: x)(1)`
	thread := new(starlark.Thread)
	thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)

	_, err := starlark.ExecFile(thread, "lambda_safety_test.go", prog, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
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

func TestDefaultUndefinedSafetyIsZero(t *testing.T) {
	b := starlark.NewBuiltin("func", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	if storedSafety := b.Safety(); storedSafety != 0 {
		t.Errorf("Default safety is not zero: got %d", storedSafety)
	}
}
