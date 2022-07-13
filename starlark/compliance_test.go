package starlark_test

import (
	"reflect"
	"sort"
	"testing"

	"go.starlark.net/starlark"
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
	anarchy := starlark.ComplianceFlags(0)
	fullCompliance := starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe

	testComplianceEnforcement(t, anarchy, fullCompliance, true)
	testComplianceEnforcement(t, fullCompliance, anarchy, false)

	disjointA := starlark.TimeSafe | starlark.IOSafe
	disjointB := starlark.MemSafe | starlark.CPUSafe
	testComplianceEnforcement(t, disjointA, disjointB, false)
	testComplianceEnforcement(t, disjointB, disjointA, false)

	common := starlark.TimeSafe | starlark.IOSafe
	symmetricallyDifferentA := starlark.MemSafe | common
	symmetricallyDifferentB := starlark.CPUSafe | common
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
