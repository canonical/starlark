package starlark_test

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/canonical/starlark/lib/json"
	starlarkmath "github.com/canonical/starlark/lib/math"
	"github.com/canonical/starlark/lib/proto"
	"github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
)

func TestCompliance(t *testing.T) {
	justCpu := starlark.CPUSafe
	justMem := starlark.MemSafe
	memAndCpu := justCpu | justMem
	unrestricted := starlark.ComplianceFlags(0)

	if err := unrestricted.Permits(memAndCpu); err != nil {
		t.Errorf("Incorrect compliance failure %v", err)
	}

	if err := justCpu.Permits(memAndCpu); err != nil {
		t.Errorf("Incorrect compliance failure: %v", err)
	}

	if memAndCpu.Permits(justCpu) == nil {
		t.Errorf("Compliance flags did not apply: missing flag not rejected")
	}

	if memAndCpu.Permits(unrestricted) == nil {
		t.Errorf("Failed to enforce compliance: restricted env allows unrestricted")
	}
}

func TestComplianceEnforcement(t *testing.T) {
	noCompliance := starlark.ComplianceFlags(0)
	fullCompliance := starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe

	// Equal compliance-sets are accepted
	testComplianceEnforcement(t, fullCompliance, fullCompliance, true)
	testComplianceEnforcement(t, fullCompliance, fullCompliance, true)

	testComplianceEnforcement(t, noCompliance, fullCompliance, true)  // Where no compliance is expected, compliance can run
	testComplianceEnforcement(t, fullCompliance, noCompliance, false) // Where full compliance is expected, no-compliance is rejected

	// Disjoint non-empty compliance sets are rejected
	disjointA := starlark.TimeSafe | starlark.IOSafe
	disjointB := starlark.MemSafe | starlark.CPUSafe
	testComplianceEnforcement(t, disjointA, disjointB, false)
	testComplianceEnforcement(t, disjointB, disjointA, false)

	// Symmetrically-different compliance sets are rejected
	common := starlark.TimeSafe | starlark.IOSafe
	symmetricallyDifferentA := starlark.MemSafe | common
	symmetricallyDifferentB := starlark.CPUSafe | common
	testComplianceEnforcement(t, symmetricallyDifferentA, symmetricallyDifferentB, false)
	testComplianceEnforcement(t, symmetricallyDifferentB, symmetricallyDifferentA, false)

	// A superset of required compliance is accepted
	testComplianceEnforcement(t, common, symmetricallyDifferentA, true)
}

func testComplianceEnforcement(t *testing.T, require, probe starlark.ComplianceFlags, expectPass bool) {
	thread := new(starlark.Thread)
	thread.RequireCompliance(require)

	const prog = `func()`
	b := starlark.NewBuiltin("func", func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("Hello, world!"), nil
	})
	b.SolemnlyDeclareCompliance(probe)
	predecls := starlark.StringDict{
		"func": b,
	}
	_, err := starlark.ExecFile(thread, "compliance_test.star", prog, predecls)
	if expectPass && err != nil {
		t.Errorf("Unexpected cancellation when testing compliance: %v", err)
	} else if !expectPass && err == nil {
		t.Errorf("Compliance enforcement did not error when expected")
	}
}

func TestMultiFunctionComplianceDeclaration(t *testing.T) {
	const n = 100
	const flags = starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe
	fns := make([]*starlark.Builtin, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("func_%d", i)
		fns = append(fns, starlark.NewBuiltin(name,
			func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				return starlark.String(name), nil
			}))
	}
	starlark.SolemnlyDeclareCompliance(flags, fns...)
	for _, fn := range fns {
		if fn.Compliance() != flags {
			t.Errorf("Failed to set compliance flags: expected %d but got %d", flags, fn.Compliance())
		}
	}
}

func TestComplianceFromNames(t *testing.T) {
	flags, err := starlark.ComplianceFromNames([]string{})
	if err != nil {
		t.Errorf("Failed to get compliance flags from list")
	}
	if flags != 0 {
		t.Errorf("Empty compliance set did not yield zero compliance flags: got %v", flags)
	}
	flags, err = starlark.ComplianceFromNames([]string{"memsafe", "cpusafe", "timesafe", "iosafe"})
	expectedFullFlags := starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe
	if err != nil {
		t.Errorf("Failed to get compliance flags from list")
	}
	if flags != expectedFullFlags {
		t.Errorf("Empty compliance set did not yield full compliance flags: got %v, expected %d", flags, expectedFullFlags)
	}
	_, err = starlark.ComplianceFromNames([]string{"memsafe", "HFJDKSLFHDJSKLFHDS"})
	if err == nil {
		t.Errorf("Invalid compliance-flag names did not yield error")
	}
}

func TestComplianceRoundTrip(t *testing.T) {
	testComplianceRoundTrip(t, []string{})
	testComplianceRoundTrip(t, []string{"memsafe"})
	testComplianceRoundTrip(t, []string{"memsafe", "iosafe"})
	testComplianceRoundTrip(t, []string{"memsafe", "cpusafe", "timesafe", "iosafe"})
}

func testComplianceRoundTrip(t *testing.T, flagNames []string) {
	flags, err := starlark.ComplianceFromNames(flagNames)
	if err != nil {
		t.Errorf("Unexpected failure computing compliance flags: %v", err)
		return
	}

	returnedNames := flags.Names()
	sort.Strings(flagNames)
	sort.Strings(returnedNames)
	if len(flagNames)|len(returnedNames) != 0 && !reflect.DeepEqual(flagNames, returnedNames) {
		t.Errorf("Round-trip flag sets are different: expected %v but got %v", flagNames, returnedNames)
	}
}

func TestThreadComplianceSetOnlyGrows(t *testing.T) {
	initialFlags := starlark.CPUSafe | starlark.MemSafe
	newFlags := starlark.IOSafe | starlark.TimeSafe
	expectedFlags := initialFlags | newFlags

	thread := new(starlark.Thread)
	thread.RequireCompliance(initialFlags)
	if thread.Compliance() != initialFlags {
		t.Errorf("Compliance flags differ from declaration: expected %v but got %v", initialFlags.Names(), thread.Compliance().Names())
	}

	thread.RequireCompliance(newFlags)

	if thread.Compliance() != expectedFlags {
		missing := thread.Compliance() &^ expectedFlags
		t.Errorf("Missing compliance flags %v, expected %v", missing.Names(), expectedFlags.Names())
	}
}

func TestLibraryCompliance(t *testing.T) {
	// Ensure that all standard functions defined by starlark are declared as fully-compliant
	const complianceAll = starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe
	universeDummyModule := &starlarkstruct.Module{Name: "universe", Members: starlark.Universe}
	mods := []*starlarkstruct.Module{universeDummyModule, json.Module, time.Module, proto.Module, starlarkmath.Module}
	for _, mod := range mods {
		for _, v := range mod.Members {
			if b, ok := v.(*starlark.Builtin); ok {
				if compliance := b.Compliance(); compliance != complianceAll {
					t.Errorf("Incorrect compliance for %s.%s: expected %s but got %s", mod.Name, b.Name(), complianceAll.Names(), compliance.Names())
				}
			}
		}
	}
}

func TestStarlarkDefinedFunctionIsCompliancePermissive(t *testing.T) {
	// Ensure that starlark-defined functions can always be run
	prog := `
def func():
	pass
func()
`
	thread := new(starlark.Thread)
	thread.RequireCompliance(starlark.CPUSafe | starlark.MemSafe)

	_, err := starlark.ExecFile(thread, "func_compliance_test.go", prog, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestLambdaComplianceIsPermissive(t *testing.T) {
	// Ensure that lambdas can always be run
	prog := `(lambda x: x)(1)`
	thread := new(starlark.Thread)
	thread.RequireCompliance(starlark.CPUSafe | starlark.MemSafe)
	_, err := starlark.ExecFile(thread, "lambda_compliance_test.go", prog, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
