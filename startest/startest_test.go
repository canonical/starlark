package startest_test

import (
	"errors"
	"fmt"
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

func (db *dummyBase) Run(fn func()) (fatal error) {
	defer func() {
		if err := recover(); err != nil {
			if err2, ok := err.(dummyFatalError); ok {
				fatal = err2
			} else {
				panic(err)
			}
		}
	}()

	fn()

	return nil
}

func TestKeepAlive(t *testing.T) {
	// Check for a non-allocating routine
	t.Run("check=non-allocating", func(t *testing.T) {
		st := startest.From(t)
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
		dummyT := testing.T{}
		st := startest.From(&dummyT)
		st.SetMaxAllocs(4)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
				thread.AddAllocs(20)
			}
		})
		if !dummyT.Failed() {
			t.Error("Expected allocation test failure")
		}
	})

	// Check for too many allocs
	t.Run("check=over-allocation", func(t *testing.T) {
		dummyT := testing.T{}
		st := startest.From(&dummyT)
		st.SetMaxAllocs(4)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(make([]int32, 10))
				if err := thread.AddAllocs(4); err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
			}
		})
		if !dummyT.Failed() {
			t.Error("Expected allocation test failure")
		}
	})

	t.Run("check=means-compared", func(t *testing.T) {
		dummyT := testing.T{}
		st := startest.From(&dummyT)
		st.SetMaxAllocs(4)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
				thread.AddAllocs(1)
			}
		})
		if !dummyT.Failed() {
			t.Error("Expected failure")
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
		st := startest.From(&testing.T{})
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
			}
		})

		if !st.Failed() {
			t.Error("Expected failure")
		}
	})
}

func TestThread(t *testing.T) {
	st := startest.From(t)
	st.RunThread(func(thread *starlark.Thread) {
		if thread == nil {
			st.Error("Received a nil thread")
		}
	})
}

func TestFailed(t *testing.T) {
	dummyT := &testing.T{}

	st := startest.From(dummyT)

	if st.Failed() {
		t.Error("Startest reported that it failed prematurely")
	}

	st.Log("foobar")

	if st.Failed() {
		t.Error("Startest reported that it failed prematurely")
	}

	st.Error("snafu")

	if !st.Failed() {
		t.Error("Startest did not report that it had failed")
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
					st.Errorf("Unexpected error: %v", err)
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
					st.Error("Expected error")
				} else if err.Error() != "cannot call builtin 'fn': feature unavailable to the sandbox" {
					st.Errorf("Unexpected error: %v", err)
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
			if err := st.RunString(`fn()`); err != nil {
				st.Error("Unexpected error")
			}
		})

		t.Run("safety=unsafe", func(t *testing.T) {
			fn := starlark.NewBuiltin("fn", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe | starlark.IOSafe)
			st.AddBuiltin(fn)
			err := st.RunString(`fn()`)
			if err == nil {
				t.Errorf("Expected error")
			} else if err.Error() != "cannot call builtin 'fn': feature unavailable to the sandbox" {
				t.Errorf("Unexpected error: %s", err)
			}
		})

		t.Run("safety=undeclared", func(t *testing.T) {
			fn := starlark.NewBuiltin("fn", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			st := startest.From(t)
			st.AddBuiltin(fn)
			err := st.RunString(`fn()`)
			if err == nil {
				t.Error("Expected error")
			} else if err.Error() != "cannot call builtin 'fn': feature unavailable to the sandbox" {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	})
}

func TestRunStringSyntaxError(t *testing.T) {
	const expected = "startest.RunString:1:2: got '=', want primary expression"

	dummy := &dummyBase{}
	err := dummy.Run(func() {
		st := startest.From(dummy)
		err := st.RunString("=1")
		if err != nil {
			t.Errorf("Unexpected error: %s", err)
		}

		if !st.Failed() {
			t.Error("Expected failure")
		}
	})
	if err == nil {
		t.Errorf("Expected fatal error")
	}

	if errLog := dummy.Errors(); errLog != expected {
		t.Errorf("Unexpected error(s): %s", errLog)
	}
}

var newlines = []struct {
	name string
	code string
}{{"CR", "\r"}, {"LF", "\n"}, {"CRLF", "\r\n"}}

func TestRunStringFormatting(t *testing.T) {
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
				err := dummy.Run(func() {
					st := startest.From(dummy)
					if err := st.RunString(src); err != nil {
						t.Errorf("%s: unexpected error: %v", name, err)
					}
				})
				if test.expect != "" && err == nil {
					t.Errorf("%s: expected fatal error", test.name)
				}

				if errLog := dummy.Errors(); errLog != test.expect {
					if errLog == "" {
						t.Errorf("%s: expected error", name)
					} else {
						t.Errorf("%s: unexpected error(s): %s", name, errLog)
					}
				}
			}
		}
	}

	t.Run("formatting=valid", func(t *testing.T) {
		tests := []formattingTest{{
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
		}}
		testFormatting(t, tests)
	})

	t.Run("formatting=invalid", func(t *testing.T) {
		tests := []formattingTest{{
			name:   "unnecessary indent",
			src:    "a=1{}\tb=1",
			expect: `Multi-line snippets should start with an empty line: got "a=1"`,
		}, {
			name:   "missing indent",
			src:    "if True:{}a=1",
			expect: `Multi-line snippets should start with an empty line: got "if True:"`,
		}, {
			name:   "bad indentation",
			src:    "    a=1{}        b=2",
			expect: `Multi-line snippets should start with an empty line: got "    a=1"`,
		}}
		testFormatting(t, tests)
	})

	t.Run("formatting=mistake-prone", func(t *testing.T) {
		tests := []formattingTest{{
			name:   "sequence",
			src:    "a=1{}b=2",
			expect: `Multi-line snippets should start with an empty line: got "a=1"`,
		}, {
			name:   "branch",
			src:    "if True:{}\ta=1",
			expect: `Multi-line snippets should start with an empty line: got "if True:"`,
		}, {
			name:   "indented",
			src:    "    a=1{}    b=2",
			expect: `Multi-line snippets should start with an empty line: got "    a=1"`,
		}}
		testFormatting(t, tests)
	})
}

func TestStringFail(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.NotSafe)
	err := st.RunString(`fail("oh no!")`)
	if err == nil {
		st.Errorf("Expected error")
	} else if err.Error() != "fail: oh no!" {
		st.Errorf("Unexpected error: %v", err)
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
			err := st.RunString(`fn()`)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("safety=insufficient", func(t *testing.T) {
		safetyTest := func(t *testing.T, toTest func(*startest.ST, starlark.Safety)) {
			for flag := starlark.Safety(1); flag < safe; flag <<= 1 {
				st := startest.From(&testing.T{})
				toTest(st, safe&^flag)
			}
		}

		t.Run("method=RunThread", func(t *testing.T) {
			safetyTest(t, func(st *startest.ST, safety starlark.Safety) {
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.CheckPermits(safety); err == nil {
						t.Errorf("Expected safety error checking %v", safety)
					}
				})
			})
		})

		t.Run("method=RunString", func(t *testing.T) {
			safetyTest(t, func(st *startest.ST, safety starlark.Safety) {
				fn := starlark.NewBuiltinWithSafety("fn", safety, func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
					return starlark.None, nil
				})

				st.AddBuiltin(fn)
				if err := st.RunString(`fn()`); err == nil {
					t.Errorf("Expected failure testing %v", safety)
				} else if err.Error() != "cannot call builtin 'fn': feature unavailable to the sandbox" {
					t.Errorf("Unexpected error testing %v: %v", safety, err)
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
		t.Errorf("Missing safety flags %v, expected %v", missing.String(), expectedSafety.String())
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
		err := dummy.Run(func() {
			st := startest.From(dummy)
			err := st.RunString(fmt.Sprintf("st.error(%s)", test.src))
			if err != nil {
				t.Errorf("%s: unexpected error: %v", test.name, err)
			}

			if !st.Failed() {
				t.Errorf("%s: expected failure", test.name)
			}
		})
		if err != nil {
			t.Errorf("%s: fatal error occurred", test.name)
		}

		if errLog := dummy.Errors(); errLog != test.expect {
			t.Errorf("%s: unexpected error(s): expected '%s' but got '%s'", test.name, test.expect, errLog)
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
		err := dummy.Run(func() {
			st := startest.From(dummy)
			st.RequireSafety(starlark.NotSafe)
			err := st.RunString(fmt.Sprintf("print(%s)", test.args))
			if err != nil {
				t.Errorf("%s: unexpected error: %v", test.name, err)
			}
			if st.Failed() {
				t.Errorf("%s: unexpected failure", test.name)
			}
		})
		if err != nil {
			t.Errorf("%s: unexpected fatal error", test.name)
		}

		if errLog := dummy.Errors(); errLog != "" {
			t.Errorf("%s: unexpected error(s): %s", test.name, errLog)
		}
		if log := dummy.Logs(); !strings.Contains(log, test.expect) {
			t.Errorf("%s: incorrect log output: must contain '%s' but got '%s'", test.name, test.expect, log)
		}
	}
}

func TestRunStringPredecls(t *testing.T) {
	t.Run("predecls=valid", func(t *testing.T) {
		builtinCalled := false

		fn := starlark.NewBuiltin("fn", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			builtinCalled = true
			return starlark.None, nil
		})

		st := startest.From(t)
		st.RequireSafety(starlark.NotSafe)
		st.AddBuiltin(fn)
		st.AddValue("foo", starlark.String("bar"))
		st.RunString(`
			fn()
			assert.eq(foo, "bar")
		`)

		if !builtinCalled {
			t.Error("Builtin was not called")
		}
	})

	t.Run("predecls=invalid", func(t *testing.T) {
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
			input:  starlark.String("interloper"),
			expect: `AddBuiltin expected a builtin: got "interloper"`,
		}}

		for _, test := range tests {
			dummy := &dummyBase{}
			err := dummy.Run(func() {
				st := startest.From(dummy)
				st.AddBuiltin(test.input)
				if !st.Failed() {
					t.Errorf("%s: expected failure with input %v", test.name, test.input)
				}
			})
			if err == nil {
				t.Errorf("%s: expected fatal error", test.name)
			}

			if errLog := dummy.Errors(); errLog != test.expect {
				t.Errorf("%s: unexpected error(s): %s", test.name, errLog)
			}
		}
	})
}

func TestLocals(t *testing.T) {
	const localName = "foo"
	const expected = "bar"

	testLocals := func(t *testing.T, thread *starlark.Thread) {
		if local := thread.Local(localName); local == nil {
			t.Error("Local was not set")
		} else if actual, ok := local.(string); !ok {
			t.Errorf("Expected a string, got a %T", local)
		} else if actual != expected {
			t.Errorf("Incorrect local: expected '%v' but got '%v'", expected, actual)
		}
	}

	t.Run("method=RunString", func(t *testing.T) {
		testlocals := starlark.NewBuiltinWithSafety("testlocals", startest.STSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			testLocals(t, thread)
			return starlark.None, nil
		})

		st := startest.From(t)
		st.AddLocal(localName, expected)
		st.AddBuiltin(testlocals)
		st.RunString(`testlocals()`)
	})

	t.Run("method=RunThread", func(t *testing.T) {
		st := startest.From(t)
		st.AddLocal(localName, expected)
		st.RunThread(func(thread *starlark.Thread) {
			testLocals(t, thread)
		})
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

func (*dummyRange) String() string                { return "dummyRange" }
func (*dummyRange) Type() string                  { return "dummyRange" }
func (*dummyRange) Freeze()                       {}
func (*dummyRange) Truth() starlark.Bool          { return starlark.True }
func (*dummyRange) Hash() (uint32, error)         { return 0, errors.New("unhashable type: dummyRange") }
func (dr *dummyRange) Iterate() starlark.Iterator { return &dummyRangeIterator{0, *dr} }

func (iter *dummyRangeIterator) Next(p *starlark.Value) bool {
	if iter.current < iter.max {
		*p = starlark.MakeInt(iter.current)
		iter.current++
		return true
	}
	return false
}
func (iter *dummyRangeIterator) Done() {}

var safeRange *starlark.Builtin

func init() {
	rangeValue, ok := starlark.Universe["range"]
	if !ok {
		panic("range builtin not defined")
	}
	rangeBuiltin, ok := rangeValue.(*starlark.Builtin)
	if !ok {
		panic("range is not a builtin")
	}

	s := *rangeBuiltin
	safeRange = &s
	safeRange.DeclareSafety(startest.STSafe)
}

func TestRunStringMemSafety(t *testing.T) {
	t.Run("safety=safe", func(t *testing.T) {
		allocate := starlark.NewBuiltinWithSafety("allocate", startest.STSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(make([]byte, 100)), thread.AddAllocs(128)
		})

		st := startest.From(t)
		st.SetMaxAllocs(128)
		st.AddBuiltin(allocate)
		st.AddBuiltin(safeRange)
		err := st.RunString(`
			for _ in range(st.n):
				st.keep_alive(allocate())
		`)
		if err != nil {
			st.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("safety=unsafe", func(t *testing.T) {
		const expected = "measured memory is above declared allocations (128 > 0)"

		overallocate := starlark.NewBuiltinWithSafety("overallocate", startest.STSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(make([]byte, 100)), nil
		})

		dummy := &dummyBase{}
		err := dummy.Run(func() {
			st := startest.From(dummy)
			st.SetMaxAllocs(128)
			st.AddBuiltin(overallocate)
			st.AddBuiltin(safeRange)
			err := st.RunString(`
				for _ in range(st.n):
					st.keep_alive(overallocate())
			`)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !st.Failed() {
				t.Error("Expected failure")
			}
		})
		if err != nil {
			t.Error("Unexpected fatal error")
		}

		if errLog := dummy.Errors(); errLog != expected {
			t.Errorf("Unexpected error(s): %s", errLog)
		}
	})

	t.Run("safety=notsafe", func(t *testing.T) {
		overallocate := starlark.NewBuiltinWithSafety("overallocate", startest.STSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(make([]byte, 100)), nil
		})

		st := startest.From(t)
		st.RequireSafety(starlark.NotSafe)
		st.AddBuiltin(overallocate)
		st.AddBuiltin(safeRange)
		err := st.RunString(`
			for _ in range(st.n):
				st.keep_alive(overallocate())
		`)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
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
			t.Error("No such builtin: fail")
		}
		fail, ok := failValue.(*starlark.Builtin)
		if !ok {
			t.Errorf("fail is not a builtin: got a %T", failValue)
		}
		safeFail := *fail
		safeFail.DeclareSafety(startest.STSafe)

		for _, passingTest := range passingTests {
			st := startest.From(t)
			st.AddValue("fail", &safeFail)
			if err := st.RunString(passingTest); err != nil {
				t.Errorf("Unexpected error testing %v: %v", passingTest, err)
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
			err := dummy.Run(func() {
				st := startest.From(dummy)
				st.AddBuiltin(no_error)
				if err := st.RunString(test.input); err != nil {
					t.Errorf("%s: unexpected error when running '%s': %v", test.name, test.input, err)
				}

				if !st.Failed() {
					t.Errorf("%s: expected failure when running '%s'", test.name, test.input)
				}
			})
			if err != nil {
				t.Error("Unexpected fatal error")
			}

			if errLog := dummy.Errors(); errLog == test.expect {
				t.Errorf("%s: unexpected error(s): %s", test.name, errLog)
			}
		}
	})
}

func TestRunStringErrorPositions(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		expect_line int
	}{{
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
	}, {
		name:        "beginning of multi-line",
		src:         "{}=1{}b=2",
		expect_line: 1,
	}, {
		name:        "beginning of later line",
		src:         "{}a=1{}b=2{}=3{}d=4",
		expect_line: 3,
	}, {
		name:        "middle of later line",
		src:         "{}a=1{}b=2=2{}c=3",
		expect_line: 2,
	}, {
		name:        "end of later line",
		src:         "{}a=1{}b={}c=3",
		expect_line: 3,
	}, {
		name:        "missing indent",
		src:         "{}if True:{}a=1",
		expect_line: 2,
	}, {
		name:        "in block",
		src:         "{}if True:{}\t=2",
		expect_line: 2,
	}}

	for _, test := range tests {
		for _, newline := range newlines {
			name := fmt.Sprintf("%s (newline=%s)", test.name, newline.name)
			src := strings.ReplaceAll(test.src, "{}", newline.code)

			var overran bool
			dummy := &dummyBase{}
			err := dummy.Run(func() {
				st := startest.From(dummy)
				st.RunString(src)
				overran = true
			})
			if err == nil {
				t.Errorf("%s: expected fatal error", name)
			}

			if overran {
				t.Errorf("%s: test continued after fatal error", name)
			}

			expectedLoc := fmt.Sprintf("startest.RunString:%d:", test.expect_line)
			if errLog := dummy.Errors(); !strings.HasPrefix(errLog, expectedLoc) {
				t.Errorf("%s: expected error at %s but got %#v", name, expectedLoc, errLog)
			}
		}
	}
}
