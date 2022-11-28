package startest_test

import (
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

func TestString(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.NotSafe)
	st.RunString(`
		print('Hello, world!')
		if False:
			print('42')
		fail('asdf')
	`)
}
