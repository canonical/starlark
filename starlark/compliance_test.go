package starlark_test

import (
	"reflect"
	"sort"
	"testing"

	"go.starlark.net/starlark"
)

func TestCompliance(t *testing.T) {
	justCpu := starlark.ComplyCPUSafe
	justMem := starlark.ComplyMemSafe
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
	anarchy := starlark.ComplianceFlags(0)
	fullCompliance := starlark.ComplyMemSafe | starlark.ComplyCPUSafe | starlark.ComplyTimeSafe | starlark.ComplyIOSafe

	testComplianceEnforcement(t, anarchy, fullCompliance, true)
	testComplianceEnforcement(t, fullCompliance, anarchy, false)

	disjointA := starlark.ComplyTimeSafe | starlark.ComplyIOSafe
	disjointB := starlark.ComplyMemSafe | starlark.ComplyCPUSafe
	testComplianceEnforcement(t, disjointA, disjointB, false)
	testComplianceEnforcement(t, disjointB, disjointA, false)

	common := starlark.ComplyTimeSafe | starlark.ComplyIOSafe
	symmetricallyDifferentA := starlark.ComplyMemSafe | common
	symmetricallyDifferentB := starlark.ComplyCPUSafe | common
	testComplianceEnforcement(t, symmetricallyDifferentA, symmetricallyDifferentB, false)
	testComplianceEnforcement(t, symmetricallyDifferentB, symmetricallyDifferentA, false)
}

func testComplianceEnforcement(t *testing.T, require, probe starlark.ComplianceFlags, expectPass bool) {
	thread := new(starlark.Thread)
	thread.RequireCompliance(require)

	const prog = `func()`
	b := starlark.NewBuiltinComplies("func", func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("Hello, world!"), nil
	}, probe)
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

func TestComplianceFromNames(t *testing.T) {
	flags, err := starlark.ComplianceFromNames([]string{})
	if err != nil {
		t.Errorf("Failed to get compliance flags from list")
	}
	if flags != 0 {
		t.Errorf("Empty compliance set did not yield zero compliance flags: got %v", flags)
	}
	flags, err = starlark.ComplianceFromNames([]string{"memsafe", "cpusafe", "timesafe", "iosafe"})
	expectedFullFlags := starlark.ComplyMemSafe | starlark.ComplyCPUSafe | starlark.ComplyTimeSafe | starlark.ComplyIOSafe
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
