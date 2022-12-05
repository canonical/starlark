package startest_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

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

			st := startest.From(&testing.T{})
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe | starlark.IOSafe)
			st.AddBuiltin(fn)
			if err := st.RunString(`fn()`); err == nil {
				t.Error("Expected error")
			} else if err.Error() != "cannot call builtin 'fn': feature unavailable to the sandbox" {
				t.Errorf("Unexpected error: %v", err)
			}
		})

		t.Run("safety=undeclared", func(t *testing.T) {
			fn := starlark.NewBuiltin("fn", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			})

			st := startest.From(&testing.T{})
			st.AddBuiltin(fn)
			err := st.RunString(`fn()`)
			if err == nil {
				t.Error("Expected error")
			} else if err.Error() != "cannot call builtin 'fn': feature unavailable to the sandbox" {
				t.Errorf("Unexpected error: %v", err)
			}

			if st.Failed() {
				t.Error("Unexpected failure")
			}
		})
	})
}

func TestStringFormatting(t *testing.T) {
	srcs := []string{"", "\n", " ", "\t", "\n\t"}
	for _, src := range srcs {
		st := startest.From(t)
		err := st.RunString(src)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func TestStringFail(t *testing.T) {
	safeFail := starlark.NewBuiltinWithSafety("fail", startest.StSafe, func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		msg, ok := args[0].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("Expected string, got %v", args[0])
		}
		return starlark.None, errors.New(string(msg))
	})

	st := startest.From(t)
	st.AddValue("fail", safeFail)
	err := st.RunString(`fail("oh no!")`)
	if err == nil {
		st.Errorf("Expected error: %v", err)
	} else if err.Error() != "oh no!" {
		st.Errorf("Unexpected error: %v", err)
	}

	if st.Failed() {
		t.Error("Unexpected error")
	}
}

func TestStringPredecls(t *testing.T) {
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
		err := st.RunString(`
			fn()
			if foo != 'bar':
				fail("foo was incorrect: expected 'bar' but got '%s'" % foo)
		`)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if !builtinCalled {
			t.Error("Builtin was not called")
		}
	})

	t.Run("predecls=invalid", func(t *testing.T) {
		st := startest.From(&testing.T{})
		st.RequireSafety(starlark.NotSafe)
		st.AddBuiltin(starlark.String("spanner"))
		err := st.RunString(``)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !st.Failed() {
			t.Error("Expected failure")
		}
	})
}

func TestRequireSafetyDefault(t *testing.T) {
	const safe = starlark.CPUSafe | starlark.IOSafe | starlark.MemSafe | starlark.TimeSafe

	st := startest.From(t)
	st.RunThread(func(thread *starlark.Thread) {
		if err := thread.CheckPermits(safe); err != nil {
			st.Error(err)
		}
	})

	for flag := starlark.Safety(1); flag < safe; flag <<= 1 {
		st := startest.From(&testing.T{})
		toCheck := safe &^ flag
		st.RunThread(func(thread *starlark.Thread) {
			if err := thread.CheckPermits(toCheck); err == nil {
				t.Errorf("Expected safety error checking %v", toCheck)
			}
		})
	}
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

func TestRunStringFormatting(t *testing.T) {
	srcs := []string{"", "\n", " ", "\t", "\n\t"}
	for _, src := range srcs {
		st := startest.From(t)
		if err := st.RunString(src); err != nil {
			st.Error(err)
		}
	}
}

func TestRunStringError(t *testing.T) {
	st := startest.From(&testing.T{})
	err := st.RunString("error('hello, world')")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !st.Failed() {
		t.Error("Expected failure")
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
			if foo != 'bar':
				st.error("foo was incorrect: expected 'bar' but got '%s'" % foo)
		`)

		if !builtinCalled {
			t.Error("Builtin was not called")
		}
	})

	t.Run("predecls=invalid", func(t *testing.T) {
		for _, val := range []starlark.Value{nil, starlark.String("interloper")} {
			st := startest.From(&testing.T{})
			st.AddBuiltin(val)

			if !st.Failed() {
				t.Errorf("Expected failure with value %v", val)
			}
		}
	})
}

func TestLocals(t *testing.T) {
	const localName = "P. Sherman"
	const expected = "42 Wallaby Way, Sydney"

	testLocals := func(t *testing.T, thread *starlark.Thread) {
		if local := thread.Local(localName); local == nil {
			t.Error("Local was not set")
		} else if actual, ok := local.(string); !ok {
			t.Errorf("Expected a string, got a %T", local)
		} else if actual != expected {
			t.Errorf("Incorrect local: expected '%v' but got '%v'", expected, actual)
		}
	}

	t.Run("entrypoint=RunString", func(t *testing.T) {
		st := startest.From(t)
		st.AddLocal(localName, expected)
		st.AddBuiltin(
			starlark.NewBuiltinWithSafety("testlocals", startest.StSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
				testLocals(t, thread)
				return starlark.None, nil
			}))
		st.RunString(`testlocals()`)
	})

	t.Run("entrypoint=RunThread", func(t *testing.T) {
		st := startest.From(t)
		st.AddLocal(localName, expected)
		st.RunThread(func(thread *starlark.Thread) {
			testLocals(t, thread)
		})
	})
}

var dummyRangeBuiltin = starlark.NewBuiltinWithSafety("rangw", startest.StSafe, func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, errors.New("Expected at least one arg, got 0")
	}
	max, ok := args[0].(starlark.Int)
	if !ok {
		return nil, fmt.Errorf("Expected int, got a %T: %v", args[0], args[0])
	}
	max64, ok := max.Int64()
	if !ok {
		return nil, fmt.Errorf("Too large")
	}
	return &dummyRange{int(max64)}, nil
})

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
func (*dummyRange) Hash() (uint32, error)         { return 0, errors.New("unhashable type: startest.ST") }
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

func TestRunStringMemSafety(t *testing.T) {
	t.Run("safety=safe", func(t *testing.T) {
		allocate := starlark.NewBuiltinWithSafety("allocate", startest.StSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(make([]byte, 100)), thread.AddAllocs(128)
		})

		st := startest.From(t)
		st.SetMaxAllocs(128)
		st.AddBuiltin(allocate)
		st.AddValue("range", dummyRangeBuiltin)
		err := st.RunString(`
			for _ in range(st.n):
				st.keep_alive(allocate())
		`)
		if err != nil {
			st.Errorf("Unexpected error: %v", err)
		}

		if st.Failed() {
			st.Error("Unexpected failure")
		}
	})

	t.Run("safety=unsafe", func(t *testing.T) {
		overallocate := starlark.NewBuiltinWithSafety("overallocate", startest.StSafe, func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(make([]byte, 100)), nil
		})

		st := startest.From(&testing.T{})
		st.SetMaxAllocs(128)
		st.AddBuiltin(overallocate)
		st.AddValue("range", dummyRangeBuiltin)
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
}
