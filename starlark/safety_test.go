package starlark_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestSafety(t *testing.T) {
	// Typical cases
	const cpuAndMemSafe = starlark.CPUSafe | starlark.MemSafe
	testSafety(t, cpuAndMemSafe, starlark.NotSafe, false)
	testSafety(t, cpuAndMemSafe, starlark.CPUSafe, false)
	testSafety(t, starlark.CPUSafe, cpuAndMemSafe, true)
	testSafety(t, starlark.NotSafe, cpuAndMemSafe, true)

	// Equal safety-sets are accepted
	testSafety(t, starlark.NotSafe, starlark.NotSafe, true)
	testSafety(t, starlark.Safe, starlark.Safe, true)

	testSafety(t, starlark.NotSafe, starlark.Safe, true)  // Where no safety is expected, something with stronger safety is permitted
	testSafety(t, starlark.Safe, starlark.NotSafe, false) // Where full safety is expected, no-safety is rejected

	// Disjoint non-empty safety sets are rejected
	const disjointA = starlark.TimeSafe | starlark.IOSafe
	const disjointB = starlark.MemSafe | starlark.CPUSafe
	testSafety(t, disjointA, disjointB, false)
	testSafety(t, disjointB, disjointA, false)

	// Symmetrically-different safety sets are rejected
	const common = starlark.TimeSafe | starlark.IOSafe
	const symmetricallyDifferentA = starlark.MemSafe | common
	const symmetricallyDifferentB = starlark.CPUSafe | common
	testSafety(t, symmetricallyDifferentA, symmetricallyDifferentB, false)
	testSafety(t, symmetricallyDifferentB, symmetricallyDifferentA, false)

	// A superset of required safety is accepted
	testSafety(t, common, symmetricallyDifferentA, true)

	// Invalid flags rejected
	const valid = starlark.Safe
	const invalid = starlark.SafetyFlags(0xbadc0de)
	testSafety(t, valid, invalid, false)
}

func testSafety(t *testing.T, require, probe starlark.SafetyFlags, expectPass bool) {
	if actual := probe.Contains(require); actual != expectPass {
		t.Errorf("safety checking did not return correct value: expected %v but got %v", expectPass, actual)
	}

	if err := probe.CheckContains(require); expectPass && err != nil {
		t.Errorf("safety checking returned unexpected error: checking that %v permits %v returned %v", require, probe, err)
	} else if !expectPass {
		if err == nil {
			t.Errorf("safety checking did not return an error when expected")
		} else if safetyErr, ok := err.(*starlark.SafetyFlagsError); !ok {
			t.Errorf("expected a safety error: got a %T", err)
		} else if expectedMissing := require &^ probe; safetyErr.Missing != expectedMissing {
			t.Errorf("incorrect missing flags reported: expected %v but got %v", expectedMissing, safetyErr.Missing)
		}
	}
}

func TestSafetyValidityChecking(t *testing.T) {
	const validSafety = starlark.MemSafe
	const invalidSafety = starlark.SafetyFlags(0xdebac1e)

	if err := validSafety.CheckValid(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := invalidSafety.CheckValid(); err == nil {
		t.Errorf("no error when checking invalid safety")
	} else if err.Error() != "internal error: invalid safety flags" {
		t.Errorf("unexpected error when checking invalid safety: %v", err)
	}
}

func TestDefaultStoredSafetyIsZero(t *testing.T) {
	b := starlark.NewBuiltin("func", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	if storedSafety := b.Safety(); storedSafety != 0 {
		t.Errorf("default safety is not zero: got %d", storedSafety)
	}
}

func TestSafetyFlagNameOrder(t *testing.T) {
	tests := map[starlark.SafetyFlags]string{
		starlark.NotSafe:  "NotSafe",
		starlark.CPUSafe:  "CPUSafe",
		starlark.MemSafe:  "MemSafe",
		starlark.TimeSafe: "TimeSafe",
		starlark.IOSafe:   "IOSafe",
		starlark.Safe:     "(CPUSafe|MemSafe|TimeSafe|IOSafe)",
	}

	maxSafetyFlag := starlark.SafetyFlags(0)
	maxSafetyFlag--
	maxSafetyFlag &^= maxSafetyFlag >> 1
	for flag := maxSafetyFlag; flag >= starlark.SafetyFlagsLimit; flag >>= 1 {
		tests[flag] = fmt.Sprintf("InvalidSafe(%d)", flag)
	}

	for safety, expected := range tests {
		if actual := safety.String(); actual != expected {
			t.Errorf("expected %s but got %s", expected, actual)
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

	builtinClosure1 := starlark.NewBuiltinWithSafety("foo", expectedClosure1Safety, base("foo"))
	builtinClosure2 := starlark.NewBuiltinWithSafety("bar", expectedClosure2Safety, base("bar"))

	if closure1Safety := builtinClosure1.Safety(); closure1Safety != expectedClosure1Safety {
		t.Errorf("incorrect safety reported on closure: expected %s but got %s", expectedClosure1Safety, closure1Safety)
	}
	if closure2Safety := builtinClosure2.Safety(); closure2Safety != expectedClosure2Safety {
		t.Errorf("incorrect safety reported on closure: expected %s but got %s", expectedClosure2Safety, closure2Safety)
	}
}

func TestFunctionSafeExecution(t *testing.T) {
	// Ensure that Starlark-defined functions can always be run
	const prog = `
def func():
	pass
func()
`
	thread := &starlark.Thread{}
	thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)

	_, err := starlark.ExecFile(thread, "func_safety_test.go", prog, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLambdaSafeExecution(t *testing.T) {
	// Ensure that lambdas can always be run
	const prog = `(lambda x: x)(1)`
	thread := &starlark.Thread{}
	thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)

	_, err := starlark.ExecFile(thread, "lambda_safety_test.go", prog, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuiltinSafeExecution(t *testing.T) {
	thread := &starlark.Thread{}
	thread.RequireSafety(starlark.CPUSafe | starlark.TimeSafe)

	fn := starlark.NewBuiltin("fn", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("foo"), nil
	})

	t.Run("BuiltinSafety=Permitted", func(t *testing.T) {
		const permittedSafety = starlark.Safe

		fn.DeclareSafety(permittedSafety)
		env := starlark.StringDict{"fn": fn}

		if _, err := starlark.ExecFile(thread, "builtin_safety_restrictions", "fn()", env); err != nil {
			t.Errorf("unexpected error executing safe builtin: %v", err)
		}
	})

	t.Run("BuiltinSafety=Forbidden", func(t *testing.T) {
		const forbiddenSafety = starlark.NotSafe

		fn.DeclareSafety(forbiddenSafety)
		env := starlark.StringDict{"fn": fn}

		if _, err := starlark.ExecFile(thread, "builtin_safety_restrictions", "fn()", env); err == nil {
			t.Errorf("expected error when trying to execute forbidden builtin")
		} else if !errors.Is(err, starlark.ErrSafety) {
			t.Errorf("unexpected error executing safe builtin: %v", err)
		}
	})

	t.Run("BuiltinSafety=Invalid", func(t *testing.T) {
		const invalidSafety = starlark.SafetyFlags(0xabcdef)

		fn.DeclareSafety(invalidSafety)

		env := starlark.StringDict{"fn": fn}
		if _, err := starlark.ExecFile(thread, "builtin_safety_restrictions", "fn()", env); err == nil {
			t.Errorf("expected error trying to evaluate builtin with invalid safety")
		} else if err.Error() != "cannot call builtin 'fn': internal error: invalid safety flags" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

type dummyCallable struct{ safety starlark.SafetyFlags }

var (
	_ starlark.Value       = &dummyCallable{}
	_ starlark.Callable    = &dummyCallable{}
	_ starlark.SafetyAware = &dummyCallable{}
)

func (dc dummyCallable) String() string        { return "" }
func (dc dummyCallable) Type() string          { return "dummyCallable" }
func (dc dummyCallable) Freeze()               {}
func (dc dummyCallable) Truth() starlark.Bool  { return false }
func (dc dummyCallable) Hash() (uint32, error) { return 0, nil }
func (dc dummyCallable) Name() string          { return "dummyCallable" }
func (dc dummyCallable) CallInternal(*starlark.Thread, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
}
func (dc *dummyCallable) Safety() starlark.SafetyFlags              { return dc.safety }
func (dc *dummyCallable) DeclareSafety(safety starlark.SafetyFlags) { dc.safety = safety }

func TestCallableSafeExecution(t *testing.T) {
	thread := &starlark.Thread{}
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
		t.Errorf("unexpected error running permitted callable %v", err)
	}

	// Forbid
	c.DeclareSafety(starlark.NotSafe)
	if _, err := starlark.ExecFile(thread, "dynamic_safety_checking", prog, env); err == nil {
		t.Errorf("expected error running dynamically-forbidden callable")
	} else if !errors.Is(err, starlark.ErrSafety) {
		t.Errorf("unexpected error running forbidden callable: %v", err)
	}

	// Repermit
	c.DeclareSafety(starlark.Safe)
	if _, err := starlark.ExecFile(thread, "dynamic_safety_checking", prog, env); err != nil {
		t.Errorf("unexpected error running dynamically re-permitted callable %v", err)
	}

	const invalidSafety = starlark.SafetyFlags(0xfedcba)
	c.DeclareSafety(invalidSafety)
	if _, err := starlark.ExecFile(thread, "dynamic_safety_checking", prog, env); err == nil {
		t.Errorf("expected invalid callable-safety to result in error")
	} else if err.Error() != "cannot call value of type 'dummyCallable': internal error: invalid safety flags" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewBuiltinWithSafety(t *testing.T) {
	fn := func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	}

	const validSafety = starlark.IOSafe | starlark.MemSafe
	if safety := starlark.NewBuiltinWithSafety("fn", validSafety, fn).Safety(); safety != validSafety {
		t.Errorf("incorrect stored safety: expected %v but got %v", validSafety, safety)
	}

	const invalidSafety = starlark.SafetyFlags(0x0ddba11)
	if safety := starlark.NewBuiltinWithSafety("fn", invalidSafety, fn).Safety(); safety != invalidSafety {
		t.Errorf("incorrect stored safety: expected %v but got %v", validSafety, safety)
	}
}

func TestBindReceiverSafety(t *testing.T) {
	const expected = starlark.SafetyFlags(0xba0bab)

	builtin := starlark.NewBuiltinWithSafety("fn", expected, func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	recv := starlark.String("foo")
	boundBuiltin := builtin.BindReceiver(recv)
	if actual := boundBuiltin.Safety(); actual != expected {
		t.Errorf("builtin with bound receiver had incorrect safety: expected %v but got %v", expected, actual)
	}
}

type dummySafetyAware struct {
	safety starlark.SafetyFlags
}

var _ starlark.SafetyAware = &dummySafetyAware{}

func (dsa *dummySafetyAware) Safety() starlark.SafetyFlags {
	return dsa.safety
}

func TestCheckSafety(t *testing.T) {
	safeThread := &starlark.Thread{}
	safeThread.RequireSafety(starlark.Safe)

	partiallySafeThread := &starlark.Thread{}
	partiallySafeThread.RequireSafety(starlark.MemSafe | starlark.CPUSafe)

	tests := []struct {
		name   string
		thread *starlark.Thread
		value  interface{}
		expect error
	}{{
		name:  "nil-thread",
		value: "unimportant",
	}, {
		name:   "not-safe-thread-not-safe-value",
		thread: &starlark.Thread{},
		value:  "not-safe",
	}, {
		name:   "not-safe-thread-safe-value",
		thread: &starlark.Thread{},
		value:  &dummySafetyAware{starlark.Safe},
	}, {
		name:   "safe-thread-not-safe-value",
		thread: safeThread,
		value:  &dummySafetyAware{},
		expect: starlark.ErrSafety,
	}, {
		name:   "safe-thread-safe-value",
		thread: safeThread,
		value:  &dummySafetyAware{starlark.Safe},
	}, {
		name:   "partially-safe-thread-unsafe-value",
		thread: partiallySafeThread,
		value:  &dummySafetyAware{},
		expect: starlark.ErrSafety,
	}, {
		name:   "safe-thread-partially-safe-value",
		thread: safeThread,
		value:  &dummySafetyAware{starlark.MemSafe | starlark.IOSafe},
		expect: starlark.ErrSafety,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := starlark.CheckSafety(test.thread, test.value)
			if err != nil {
				if test.expect == nil || !errors.Is(err, test.expect) {
					t.Errorf("unexpected error: %v", err)
				}
			} else if test.expect != nil {
				t.Errorf("no error returned, expected: %v", test.expect)
			}
		})
	}
}
