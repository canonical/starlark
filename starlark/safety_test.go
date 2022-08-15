package starlark_test

import (
	"testing"

	"github.com/canonical/starlark/lib/json"
	starlarkmath "github.com/canonical/starlark/lib/math"
	"github.com/canonical/starlark/lib/proto"
	"github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
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

func TestSafetyEnforcement(t *testing.T) {
	noSafety := starlark.SafetyFlags(0)
	fullSafety := starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe

	// Equal safety-sets are accepted
	testSafetyEnforcement(t, fullSafety, fullSafety, true)
	testSafetyEnforcement(t, fullSafety, fullSafety, true)

	testSafetyEnforcement(t, noSafety, fullSafety, true)  // Where no safety is expected, safety can run
	testSafetyEnforcement(t, fullSafety, noSafety, false) // Where full safety is expected, no-safety is rejected

	// Disjoint non-empty safety sets are rejected
	disjointA := starlark.TimeSafe | starlark.IOSafe
	disjointB := starlark.MemSafe | starlark.CPUSafe
	testSafetyEnforcement(t, disjointA, disjointB, false)
	testSafetyEnforcement(t, disjointB, disjointA, false)

	// Symmetrically-different safety sets are rejected
	common := starlark.TimeSafe | starlark.IOSafe
	symmetricallyDifferentA := starlark.MemSafe | common
	symmetricallyDifferentB := starlark.CPUSafe | common
	testSafetyEnforcement(t, symmetricallyDifferentA, symmetricallyDifferentB, false)
	testSafetyEnforcement(t, symmetricallyDifferentB, symmetricallyDifferentA, false)

	// A superset of required safety is accepted
	testSafetyEnforcement(t, common, symmetricallyDifferentA, true)
}

func testSafetyEnforcement(t *testing.T, require, probe starlark.SafetyFlags, expectPass bool) {
	thread := new(starlark.Thread)
	thread.RequireSafety(require)

	const prog = `func()`
	b := starlark.NewBuiltin("func", func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("Hello, world!"), nil
	})
	b.DeclareSafety(probe)
	predecls := starlark.StringDict{
		"func": b,
	}
	_, err := starlark.ExecFile(thread, "safety_test.star", prog, predecls)
	if expectPass && err != nil {
		t.Errorf("Unexpected cancellation when testing safety: %v", err)
	} else if !expectPass && err == nil {
		t.Errorf("Safety enforcement did not error when expected")
	}
}

func TestThreadSafetySetOnlyGrows(t *testing.T) {
	initialFlags := starlark.CPUSafe | starlark.MemSafe
	newFlags := starlark.IOSafe | starlark.TimeSafe
	expectedFlags := initialFlags | newFlags

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

func TestLibrarySafety(t *testing.T) {
	// Ensure that all standard functions defined by starlark are declared as fully-safe
	const safetyAll = starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe
	universeDummyModule := &starlarkstruct.Module{Name: "universe", Members: starlark.Universe}
	mods := []*starlarkstruct.Module{universeDummyModule, json.Module, time.Module, proto.Module, starlarkmath.Module}
	for _, mod := range mods {
		for _, v := range mod.Members {
			if b, ok := v.(*starlark.Builtin); ok {
				if safety := b.Safety(); safety != safetyAll {
					t.Errorf("Incorrect safety for %s.%s: expected %s but got %s", mod.Name, b.Name(), safetyAll.Names(), safety.Names())
				}
			}
		}
	}
}

func TestStarlarkDefinedFunctionSafetyIsPermissive(t *testing.T) {
	// Ensure that starlark-defined functions can always be run
	prog := `
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
	prog := `(lambda x: x)(1)`
	thread := new(starlark.Thread)
	thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
	_, err := starlark.ExecFile(thread, "lambda_safety_test.go", prog, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
