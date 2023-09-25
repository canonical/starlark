package startest_test

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

type dummyBase struct {
	failed bool
	errors *strings.Builder
	logs   *strings.Builder
}

var _ startest.TestBase = &dummyBase{}

func (db *dummyBase) Error(errs ...interface{}) {
	db.failed = true

	if db.errors == nil {
		db.errors = &strings.Builder{}
	}

	if db.errors.Len() != 0 {
		db.errors.WriteRune('\n')
	}
	for i, err := range errs {
		if i > 0 {
			db.errors.WriteRune(' ')
		}
		db.errors.WriteString(fmt.Sprintf("%v", err))
	}
}

func (db *dummyBase) Errorf(format string, args ...interface{}) {
	db.failed = true

	if db.errors == nil {
		db.errors = &strings.Builder{}
	}

	if db.errors.Len() != 0 {
		db.errors.WriteRune('\n')
	}
	db.errors.WriteString(fmt.Errorf(format, args...).Error())
}

type dummyFatalError struct{}

func (e dummyFatalError) Error() string { return "dummy fatal error" }

func (db *dummyBase) Fatal(args ...interface{}) {
	db.Error(args...)
	panic(dummyFatalError{})
}

func (db *dummyBase) Fatalf(format string, args ...interface{}) {
	db.Errorf(format, args...)
	panic(dummyFatalError{})
}

func (db *dummyBase) Log(args ...interface{}) {
	if db.logs == nil {
		db.logs = &strings.Builder{}
	}

	if db.logs.Len() != 0 {
		db.logs.WriteRune('\n')
	}
	for i, arg := range args {
		if i > 0 {
			db.logs.WriteRune(' ')
		}
		db.logs.WriteString(fmt.Sprintf("%v", arg))
	}
}

func (db *dummyBase) Logf(format string, args ...interface{}) {
	if db.logs == nil {
		db.logs = &strings.Builder{}
	}

	if db.logs.Len() != 0 {
		db.logs.WriteRune('\n')
	}
	db.logs.WriteString(fmt.Sprintf(format, args...))
}

func (db *dummyBase) Failed() bool { return db.failed }

func (db *dummyBase) Errors() string {
	if db.errors == nil {
		return ""
	}
	return db.errors.String()
}

func (db *dummyBase) Logs() string {
	if db.logs == nil {
		return ""
	}
	return db.logs.String()
}

func TestKeepAlive(t *testing.T) {
	// Check for a non-allocating routine
	t.Run("check=non-allocating", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(nil)
			}
		})
	})

	// Check for exact measuring
	t.Run("check=exact", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(4)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
				thread.AddAllocs(4)
			}
		})
	})

	// Check for over estimations
	t.Run("check=over-estimation", func(t *testing.T) {
		const expected = "declared allocations are above maximum (20 > 4)"

		dummy := &dummyBase{}
		st := startest.From(dummy)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(4)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
				thread.AddAllocs(20)
			}
		})
		if errLog := dummy.Errors(); errLog != expected {
			t.Errorf("unexpected error(s): %s", errLog)
		}
	})

	// Check for too many allocs
	t.Run("check=over-allocation", func(t *testing.T) {
		const expected = "measured memory is above maximum"

		dummy := &dummyBase{}
		st := startest.From(dummy)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(4)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(make([]int32, 10))
				if err := thread.AddAllocs(4); err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
			}
		})
		if errLog := dummy.Errors(); !strings.HasPrefix(errLog, expected) {
			t.Errorf("unexpected error(s): %s", errLog)
		}
	})

	t.Run("check=means-compared", func(t *testing.T) {
		const expected = "measured memory is above declared allocations (4 > 1)"

		dummy := &dummyBase{}
		st := startest.From(dummy)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(4)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
				thread.AddAllocs(1)
			}
		})
		if errLog := dummy.Errors(); errLog != expected {
			t.Errorf("unexpected error(s): %s", errLog)
		}
	})

	t.Run("check=not-safe", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.NotSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
			}
		})
	})

	t.Run("check=not-safe-capped-allocs", func(t *testing.T) {
		const expected = "measured memory is above maximum (4 > 0)\nmeasured memory is above declared allocations (4 > 0)"

		dummy := &dummyBase{}
		st := startest.From(dummy)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
			}
		})

		if !st.Failed() {
			t.Error("expected failure")
		}
		if errLog := dummy.Errors(); errLog != expected {
			t.Errorf("unexpected error(s): %s", errLog)
		}
	})
}

func TestStepBounding(t *testing.T) {
	t.Run("steps=safe", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxExecutionSteps(10)

		st.RunString(`
			for _ in st.ntimes():
				pass
		`)
	})

	t.Run("steps=not-safe", func(t *testing.T) {
		expected := regexp.MustCompile(`execution steps are above maximum \(\d+ > 1\)`)

		dummy := &dummyBase{}
		st := startest.From(dummy)
		st.SetMaxExecutionSteps(1)
		st.RunString(`
			i = 0
			for _ in st.ntimes():
				i += 1
				i += 1
		`)

		if !st.Failed() {
			t.Error("expected failure")
		}
		if errLog := dummy.Errors(); !expected.Match([]byte(errLog)) {
			t.Errorf("unexpected error(s): %s", errLog)
		}
	})
}

func TestThread(t *testing.T) {
	st := startest.From(t)
	st.RunThread(func(thread *starlark.Thread) {
		if thread == nil {
			st.Error("received a nil thread")
		}
	})
}

func TestFailed(t *testing.T) {
	dummy := &dummyBase{}
	st := startest.From(dummy)

	if st.Failed() {
		t.Error("startest reported that it failed prematurely")
	}

	st.Log("foobar")

	if st.Failed() {
		t.Error("startest reported that it failed prematurely")
	}
	if log := dummy.Logs(); log != "foobar" {
		t.Errorf("unexpected log output: %s", log)
	}
	if errLog := dummy.Errors(); errLog != "" {
		t.Errorf("unexpected error logged: %s", errLog)
	}

	st.Error("snafu")

	if !st.Failed() {
		t.Error("startest did not report that it had failed")
	}
	if log := dummy.Logs(); log != "foobar" {
		t.Errorf("unexpected log output: %s", log)
	}
	if errLog := dummy.Errors(); errLog != "snafu" {
		t.Errorf("unexpected error logged: %s", errLog)
	}
}

func TestRequireSafety(t *testing.T) {
	t.Run("method=RunThread", func(t *testing.T) {
		t.Run("safety=safe", func(t *testing.T) {
			builtin := starlark.NewBuiltinWithSafety("fn", starlark.MemSafe|starlark.IOSafe, func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.IOSafe)
			st.RunThread(func(thread *starlark.Thread) {
				if _, err := starlark.Call(thread, builtin, nil, nil); err != nil {
					st.Errorf("unexpected error: %v", err)
				}
			})
		})

		t.Run("safety=unsafe", func(t *testing.T) {
			builtin := starlark.NewBuiltin("fn", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.IOSafe)
			st.RunThread(func(thread *starlark.Thread) {
				if _, err := starlark.Call(thread, builtin, nil, nil); err == nil {
					st.Error("expected error")
				} else if err.Error() != "cannot call builtin 'fn': feature unavailable to the sandbox" {
					st.Errorf("unexpected error: %v", err)
				}
			})
		})
	})

	t.Run("method=RunString", func(t *testing.T) {
		t.Run("safety=safe", func(t *testing.T) {
			fn := starlark.NewBuiltinWithSafety("fn", starlark.MemSafe|starlark.TimeSafe, func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.AddBuiltin(fn)
			if ok := st.RunString(`fn()`); !ok {
				st.Error("RunString returned false")
			}
		})

		t.Run("safety=unsafe", func(t *testing.T) {
			const expected = "cannot call builtin 'fn': feature unavailable to the sandbox"

			fn := starlark.NewBuiltin("fn", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			dummy := &dummyBase{}
			st := startest.From(dummy)
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe | starlark.IOSafe)
			st.AddBuiltin(fn)

			if ok := st.RunString(`fn()`); ok {
				t.Errorf("RunString returned true")
			}
			if errLog := dummy.Errors(); errLog != expected {
				t.Errorf("unexpected error(s): %#v", errLog)
			}
		})

		t.Run("safety=undeclared", func(t *testing.T) {
			const expected = "cannot call builtin 'fn': feature unavailable to the sandbox"
			fn := starlark.NewBuiltin("fn", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			dummy := &dummyBase{}
			st := startest.From(dummy)
			st.AddBuiltin(fn)
			if ok := st.RunString(`fn()`); ok {
				t.Error("RunString returned true")
			}
			if errLog := dummy.Errors(); errLog != expected {
				t.Errorf("unexpected error(s): %#v", errLog)
			}
		})
	})
}

var newlines = []struct {
	name string
	code string
}{{"CR", "\r"}, {"LF", "\n"}, {"CRLF", "\r\n"}}

func TestRunStringSyntax(t *testing.T) {
	type formattingTest struct {
		name   string
		src    string
		expect string
	}

	testFormatting := func(t *testing.T, tests []formattingTest) {
		for _, test := range tests {
			for _, newline := range newlines {
				name := fmt.Sprintf("%s (newline=%s)", test.name, newline.name)
				src := strings.ReplaceAll(test.src, "{}", newline.code)

				dummy := &dummyBase{}
				st := startest.From(dummy)
				ok := st.RunString(src)
				if ok != (test.expect == "") {
					t.Errorf("%s: RunString returned %t", name, ok)
				}

				if errLog := dummy.Errors(); !strings.Contains(errLog, test.expect) {
					if errLog == "" {
						t.Errorf("%s: expected error", name)
					} else {
						t.Errorf("%s: unexpected error(s): %#v", name, errLog)
					}
				}
			}
		}
	}

	t.Run("formatting=valid", func(t *testing.T) {
		tests := []formattingTest{{
			name: "empty",
			src:  "",
		}, {
			name: "trivial",
			src:  "a=1",
		}, {
			name: "preceding newline",
			src:  "{}a=1",
		}, {
			name: "surrounding newlines",
			src:  "{}a=1{}",
		}, {
			name: "if block with space indent",
			src:  "{}if True:{} a=1",
		}, {
			name: "if block with tab indent",
			src:  "{}if True:{}\ta=1",
		}, {
			name: "tab-indented if block with tab indent",
			src:  "{}\tif True:{}\t\ta=1",
		}, {
			name: "space-indented if block with tab indent",
			src:  "{}    if True:{}        a=1",
		}, {
			name: "mixed-indented if block with tab indent",
			src:  "{}  \t  if True:{}  \t      a=1",
		}}
		testFormatting(t, tests)
	})

	t.Run("formatting=mistake-prone", func(t *testing.T) {
		tests := []formattingTest{{
			name: "sequence",
			src:  "a=1{}b=2",
		}, {
			name: "branch",
			src:  "if True:{}\ta=1",
		}, {
			name: "indented",
			src:  "\ta=1{}\tb=2",
		}}
		testFormatting(t, tests)
	})

	t.Run("formatting=invalid", func(t *testing.T) {
		tests := []formattingTest{{
			name:   "unnecessary indent",
			src:    "a=1{}\tb=1",
			expect: "got indent, want primary expression",
		}, {
			name:   "indented unnecessary indent",
			src:    "\ta=1{}\t\tb=2",
			expect: "got indent, want primary expression",
		}, {
			name:   "missing indent",
			src:    "if True:{}a=1",
			expect: "got identifier, want indent",
		}}
		testFormatting(t, tests)
	})
}

func TestStringFail(t *testing.T) {
	const expected = "fail: oh no!"

	dummy := &dummyBase{}
	st := startest.From(dummy)
	st.RequireSafety(starlark.NotSafe)
	if ok := st.RunString(`fail("oh no!")`); ok {
		st.Errorf("RunString returned true")
	}
	if errLog := dummy.Errors(); errLog != expected {
		st.Errorf("unexpected error(s): %#v", errLog)
	}
}

func TestRequireSafetyDefault(t *testing.T) {
	const safe = starlark.CPUSafe | starlark.IOSafe | starlark.MemSafe | starlark.TimeSafe

	t.Run("safety=safe", func(t *testing.T) {
		t.Run("method=RunThread", func(t *testing.T) {
			st := startest.From(t)
			st.RunThread(func(thread *starlark.Thread) {
				if err := thread.CheckPermits(safe); err != nil {
					st.Error(err)
				}
			})
		})

		t.Run("method=RunString", func(t *testing.T) {
			fn := starlark.NewBuiltinWithSafety("fn", safe, func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			st := startest.From(t)
			st.AddBuiltin(fn)
			if ok := st.RunString(`fn()`); !ok {
				t.Errorf("RunString returned false")
			}
		})
	})

	t.Run("safety=insufficient", func(t *testing.T) {
		safetyTest := func(t *testing.T, toTest func(starlark.Safety)) {
			for flag := starlark.Safety(1); flag < safe; flag <<= 1 {
				toTest(safe &^ flag)
			}
		}

		t.Run("method=RunThread", func(t *testing.T) {
			safetyTest(t, func(safety starlark.Safety) {
				st := startest.From(t)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.CheckPermits(safety); err == nil {
						t.Errorf("expected safety error checking %v", safety)
					}
				})
			})
		})

		t.Run("method=RunString", func(t *testing.T) {
			safetyTest(t, func(safety starlark.Safety) {
				const expected = "cannot call builtin 'fn': feature unavailable to the sandbox"

				fn := starlark.NewBuiltinWithSafety("fn", safety, func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
					return starlark.None, nil
				})

				dummy := &dummyBase{}
				st := startest.From(dummy)
				st.AddBuiltin(fn)
				if ok := st.RunString(`fn()`); ok {
					t.Errorf("RunString returned true testing %v", safety)
				}
				if errLog := dummy.Errors(); errLog != expected {
					t.Errorf("unexpected error(s) testing %v: %#v", safety, errLog)
				}
			})
		})
	})
}

func TestRequireSafetyDoesNotUnsetFlags(t *testing.T) {
	const initialSafety = starlark.CPUSafe
	const newSafety = starlark.IOSafe | starlark.TimeSafe
	const expectedSafety = initialSafety | newSafety

	st := startest.From(t)
	st.RequireSafety(initialSafety)
	st.RequireSafety(newSafety)

	if safety := startest.STSafety(st); safety != expectedSafety {
		missing := safety &^ expectedSafety
		t.Errorf("missing safety flags %v, expected %v", missing.String(), expectedSafety.String())
	}

	st.RunThread(func(thread *starlark.Thread) {
		if err := thread.CheckPermits(expectedSafety); err != nil {
			st.Error(err)
		}
	})
}

func TestRunStringError(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		expect string
	}{{
		name:   "empty",
		src:    "",
		expect: "",
	}, {
		name:   "string",
		src:    "'hello, world'",
		expect: "hello, world",
	}, {
		name:   "int",
		src:    "1",
		expect: "1",
	}, {
		name:   "list",
		src:    "['a', 'b']",
		expect: `["a", "b"]`,
	}, {
		name:   "multiple-strings",
		src:    "'hello', 'world'",
		expect: "hello world",
	}}

	for _, test := range tests {
		dummy := &dummyBase{}
		st := startest.From(dummy)
		ok := st.RunString(fmt.Sprintf("st.error(%s)", test.src))
		if !ok {
			t.Errorf("%s: RunString returned false", test.name)
		}
		if !st.Failed() {
			t.Errorf("%s: expected failure", test.name)
		}

		if errLog := dummy.Errors(); errLog != test.expect {
			t.Errorf("%s: unexpected error(s): expected %#v but got %#v", test.name, test.expect, errLog)
		}
		if log := dummy.Logs(); log != "" {
			t.Errorf("%s: unexpected log output: %s", test.name, log)
		}
	}
}

func TestRunStringPrint(t *testing.T) {
	tests := []struct {
		name   string
		args   string
		expect string
	}{{
		name:   "empty",
		args:   "",
		expect: "",
	}, {
		name:   "string",
		args:   "'hello, world'",
		expect: "hello, world",
	}, {
		name:   "int",
		args:   "1",
		expect: "1",
	}, {
		name:   "list",
		args:   "['a', 'b']",
		expect: `["a", "b"]`,
	}, {
		name:   "multiple-strings",
		args:   "'hello', 'world'",
		expect: "hello world",
	}}

	for _, test := range tests {
		dummy := &dummyBase{}
		st := startest.From(dummy)
		st.RequireSafety(starlark.NotSafe)
		ok := st.RunString(fmt.Sprintf("print(%s)", test.args))
		if !ok {
			t.Errorf("%s: RunString returned false", test.name)
		}
		if st.Failed() {
			t.Errorf("%s: unexpected failure", test.name)
		}

		if errLog := dummy.Errors(); errLog != "" {
			t.Errorf("%s: unexpected error(s): %#v", test.name, errLog)
		}
		if log := dummy.Logs(); !strings.Contains(log, test.expect) {
			t.Errorf("%s: incorrect log output: must contain %#v but got %#v", test.name, test.expect, log)
		}
	}
}

func TestRunStringPredecls(t *testing.T) {
	t.Run("method=AddValue", func(t *testing.T) {
		t.Run("input=valid", func(t *testing.T) {
			tests := []struct {
				name  string
				value starlark.Value
				repr  string
			}{{
				name:  "string",
				value: starlark.String("foo"),
				repr:  `"foo"`,
			}, {
				name: "builtin",
				value: starlark.NewBuiltin("value", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
					return starlark.None, nil
				}),
				repr: "<built-in function value>",
			}}

			for _, test := range tests {
				st := startest.From(t)
				st.RequireSafety(starlark.NotSafe)
				st.AddValue("value", test.value)
				if ok := st.RunString(fmt.Sprintf("assert.eq(repr(value), %#v)", test.repr)); !ok {
					t.Errorf("%s: RunString returned false", test.name)
				}
			}
		})

		t.Run("input=invalid", func(t *testing.T) {
			const expected = "AddValue expected a value: got <nil>"

			dummy := &dummyBase{}
			st := startest.From(dummy)
			st.AddValue("value", nil)
			if !st.Failed() {
				t.Errorf("expected failure adding nil value")
			}

			if errLog := dummy.Errors(); errLog != expected {
				t.Errorf("unexpected error(s): %#v", errLog)
			}
		})
	})
	t.Run("method=AddBuiltin", func(t *testing.T) {
		t.Run("input=valid", func(t *testing.T) {
			builtinCalled := false

			fn := starlark.NewBuiltin("fn", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
				builtinCalled = true
				return starlark.None, nil
			})

			st := startest.From(t)
			st.RequireSafety(starlark.NotSafe)
			st.AddBuiltin(fn)
			if ok := st.RunString(`fn()`); !ok {
				t.Errorf("RunString returned false")
			}

			if !builtinCalled {
				t.Error("builtin was not called")
			}
		})

		t.Run("input=invalid", func(t *testing.T) {
			tests := []struct {
				name   string
				input  starlark.Value
				expect string
			}{{
				name:   "nil",
				input:  nil,
				expect: "AddBuiltin expected a builtin: got <nil>",
			}, {
				name:   "string",
				input:  starlark.String("spanner"),
				expect: `AddBuiltin expected a builtin: got "spanner"`,
			}}

			for _, test := range tests {
				dummy := &dummyBase{}
				st := startest.From(dummy)
				st.AddBuiltin(test.input)
				if !st.Failed() {
					t.Errorf("%s: expected failure with input %v", test.name, test.input)
				}

				if errLog := dummy.Errors(); errLog != test.expect {
					t.Errorf("%s: unexpected error(s): %#v", test.name, errLog)
				}
			}
		})
	})
}

func TestLocals(t *testing.T) {
	const localName = "foo"
	const expected = "bar"

	testLocals := func(t *testing.T, thread *starlark.Thread) {
		if local := thread.Local(localName); local == nil {
			t.Error("local was not set")
		} else if actual, ok := local.(string); !ok {
			t.Errorf("expected a string, got a %T", local)
		} else if actual != expected {
			t.Errorf("incorrect local: expected '%v' but got '%v'", expected, actual)
		}
	}

	t.Run("method=RunThread", func(t *testing.T) {
		st := startest.From(t)
		st.AddLocal(localName, expected)
		st.RunThread(func(thread *starlark.Thread) {
			testLocals(t, thread)
		})
	})

	t.Run("method=RunString", func(t *testing.T) {
		testlocals := starlark.NewBuiltinWithSafety("testlocals", startest.STSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			testLocals(t, thread)
			return starlark.None, nil
		})

		st := startest.From(t)
		st.AddLocal(localName, expected)
		st.AddBuiltin(testlocals)
		if ok := st.RunString(`testlocals()`); !ok {
			t.Error("RunString returned false")
		}
	})
}

type dummyRange struct{ max int }
type dummyRangeIterator struct {
	current int
	dummyRange
}

var _ starlark.Value = &dummyRange{}
var _ starlark.Iterable = &dummyRange{}
var _ starlark.Iterator = &dummyRangeIterator{}

func (dr *dummyRange) String() string             { return "dummyRange" }
func (dr *dummyRange) Type() string               { return "dummyRange" }
func (dr *dummyRange) Freeze()                    {}
func (dr *dummyRange) Truth() starlark.Bool       { return starlark.True }
func (dr *dummyRange) Hash() (uint32, error)      { return 0, errors.New("unhashable type: dummyRange") }
func (dr *dummyRange) Iterate() starlark.Iterator { return &dummyRangeIterator{0, *dr} }

func (iter *dummyRangeIterator) Next(p *starlark.Value) bool {
	if iter.current < iter.max {
		*p = starlark.MakeInt(iter.current)
		iter.current++
		return true
	}
	return false
}

func (iter *dummyRangeIterator) Done()      {}
func (iter *dummyRangeIterator) Err() error { return nil }

func TestRunStringMemSafety(t *testing.T) {
	t.Run("safety=safe", func(t *testing.T) {
		allocateResultSize := starlark.EstimateSize(starlark.Tuple{}) +
			starlark.EstimateMakeSize(starlark.Tuple{}, 100)
		allocate := starlark.NewBuiltinWithSafety("allocate", startest.STSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			if err := thread.AddAllocs(allocateResultSize); err != nil {
				return nil, err
			}
			return make(starlark.Tuple, 100), nil
		})

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(uint64(allocateResultSize))
		st.AddBuiltin(allocate)
		ok := st.RunString(`
			for _ in st.ntimes():
				st.keep_alive(allocate())
		`)
		if !ok {
			st.Errorf("RunString returned false")
		}
	})

	t.Run("safety=unsafe", func(t *testing.T) {
		const expected = "measured memory is above declared allocations"

		overallocateResultSize := starlark.EstimateSize(starlark.Tuple{}) +
			starlark.EstimateMakeSize(starlark.Tuple{}, 100)
		overallocate := starlark.NewBuiltinWithSafety("overallocate", startest.STSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return make(starlark.Tuple, 100), nil
		})

		dummy := &dummyBase{}
		st := startest.From(dummy)
		st.SetMaxAllocs(uint64(overallocateResultSize * 2)) // test correct error when within max
		st.AddBuiltin(overallocate)
		ok := st.RunString(`
			for _ in st.ntimes():
				st.keep_alive(overallocate())
		`)
		if !ok {
			t.Errorf("RunString returned false")
		}

		if !st.Failed() {
			t.Error("expected failure")
		}

		if errLog := dummy.Errors(); !strings.HasPrefix(errLog, expected) {
			t.Errorf("unexpected error(s): %#v", errLog)
		}
	})

	t.Run("safety=notsafe", func(t *testing.T) {
		overallocate := starlark.NewBuiltinWithSafety("overallocate", startest.STSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return make(starlark.Tuple, 100), nil
		})

		st := startest.From(t)
		st.RequireSafety(starlark.NotSafe)
		st.AddBuiltin(overallocate)
		ok := st.RunString(`
			for _ in st.ntimes():
				st.keep_alive(overallocate())
		`)
		if !ok {
			t.Errorf("RunString returned false")
		}
	})
}

func TestAssertModuleIntegration(t *testing.T) {
	t.Run("assertions=pass", func(t *testing.T) {
		passingTests := []string{
			`assert.eq(1,1)`,
			`assert.ne(1,2)`,
			`assert.true('str')`,
			`assert.lt(1,2)`,
			`assert.contains([1,2],2)`,
			`assert.fails(lambda: fail("don't touch anything"), "fail: don't touch anything")`,
		}

		failValue, ok := starlark.Universe["fail"]
		if !ok {
			t.Error("no such builtin: fail")
		}
		fail, ok := failValue.(*starlark.Builtin)
		if !ok {
			t.Errorf("fail is not a builtin: got a %T", failValue)
		}
		safeFail := *fail
		safeFail.DeclareSafety(startest.STSafe)

		for _, passingTest := range passingTests {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe) // TODO: remove this once full safety reached
			st.AddValue("fail", &safeFail)
			if ok := st.RunString(passingTest); !ok {
				t.Errorf("RunString returned false")
			}
		}
	})

	t.Run("assertions=fail", func(t *testing.T) {
		no_error := starlark.NewBuiltinWithSafety("no_error", startest.STSafe, func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.None, nil
		})

		tests := []struct {
			name   string
			input  string
			expect string
		}{{
			name:   "fail",
			input:  `assert.fail('oh no')`,
			expect: "oh no",
		}, {
			name:   "eq",
			input:  `assert.eq(1,2)`,
			expect: "1 != 2",
		}, {
			name:   "ne",
			input:  `assert.ne(1,1)`,
			expect: "1 == 1",
		}, {
			name:   "true",
			input:  `assert.true('')`,
			expect: "assertion failed",
		}, {
			name:   "lt",
			input:  `assert.lt(1,1)`,
			expect: "1 is not less than 1",
		}, {
			name:   "contains",
			input:  `assert.contains([],1)`,
			expect: "[] does not contain 1",
		}, {
			name:   "fails",
			input:  `assert.fails(lambda: no_error(), 'some expected error')`,
			expect: `evaluation succeeded unexpectedly (want error matching "some expected error")`,
		}}

		for _, test := range tests {
			dummy := &dummyBase{}
			st := startest.From(dummy)
			st.RequireSafety(starlark.MemSafe) // TODO: remove this once full safety reached
			st.AddBuiltin(no_error)
			if ok := st.RunString(test.input); !ok {
				t.Errorf("%s: RunString returned false on '%s'", test.name, test.input)
			}

			if !st.Failed() {
				t.Errorf("%s: expected failure when running '%s'", test.name, test.input)
			}

			if errLog := dummy.Errors(); errLog == test.expect {
				t.Errorf("%s: unexpected error(s): %#v", test.name, errLog)
			}
		}
	})
}

func TestRunStringErrorPositions(t *testing.T) {
	type test struct {
		name        string
		src         string
		expect_line int
	}

	tests := []test{{
		name:        "beginning of sole line",
		src:         "=1",
		expect_line: 1,
	}, {
		name:        "middle of sole line",
		src:         "a=1=1",
		expect_line: 1,
	}, {
		name:        "end of sole line",
		src:         "a=",
		expect_line: 1,
	}, {
		name:        "beginning of sole line after blanks",
		src:         "{}{}{}{}{}{}{}=1",
		expect_line: 7,
	}}

	multiLineTests := []test{{
		name:        "beginning of multi-line",
		src:         "=1{}b=2",
		expect_line: 1,
	}, {
		name:        "beginning of later line",
		src:         "a=1{}b=2{}=3{}d=4",
		expect_line: 3,
	}, {
		name:        "middle of later line",
		src:         "a=1{}b=2=2{}c=3",
		expect_line: 2,
	}, {
		name:        "end of later line",
		src:         "a=1{}b={}c=3",
		expect_line: 3,
	}, {
		name:        "missing indent",
		src:         "if True:{}a=1",
		expect_line: 2,
	}, {
		name:        "in block",
		src:         "if True:{}\t=2",
		expect_line: 2,
	}}

	for _, test := range multiLineTests {
		tests = append(tests, test)

		prettyTest := test
		prettyTest.name = fmt.Sprintf("%s (with pretty newline)", test.name)
		prettyTest.src = "{}" + prettyTest.src
		tests = append(tests, prettyTest)
	}

	for _, test := range tests {
		for _, newline := range newlines {
			name := fmt.Sprintf("%s (newline=%s)", test.name, newline.name)
			src := strings.ReplaceAll(test.src, "{}", newline.code)

			dummy := &dummyBase{}
			st := startest.From(dummy)
			ok := st.RunString(src)
			if ok {
				t.Errorf("%s: RunString returned true", name)
			}

			expectedLoc := fmt.Sprintf("startest.RunString:%d:", test.expect_line)
			if errLog := dummy.Errors(); !strings.HasPrefix(errLog, expectedLoc) {
				t.Errorf("%s: expected error at %s but got %#v", name, expectedLoc, errLog)
			}
		}
	}
}
