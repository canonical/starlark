package starlark_test

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestDefaultAllocMaxIsUnbounded(t *testing.T) {
	thread := &starlark.Thread{}

	if err := thread.CheckAllocs(1); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if _, err := starlark.ExecFile(thread, "default_allocs_test", "", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := thread.AddAllocs(math.MaxInt64); err != nil {
			t.Errorf("unexpected error: %v", err)
			break
		}
	}
}

func TestCheckAllocs(t *testing.T) {
	thread := &starlark.Thread{}
	thread.SetMaxAllocs(1000)

	if err := thread.CheckAllocs(500); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if allocs := thread.Allocs(); allocs != 0 {
		t.Errorf("CheckAllocs recorded allocations: expected 0 but got %v", allocs)
	}

	if err := thread.CheckAllocs(2000); err == nil {
		t.Errorf("expected error")
	} else {
		expected := &starlark.AllocsSafetyError{}
		if !errors.As(err, &expected) {
			t.Errorf("unexpected error: %v", err)
		} else if allocs := thread.Allocs(); allocs != 0 {
			t.Errorf("CheckAllocs recorded allocations: expected 0 but got %v", allocs)
		}
	}

	if _, err := starlark.ExecFile(thread, "alloc_cancel_test", "", nil); err != nil {
		t.Errorf("unexpected cancellation: %v", err)
	}
}

func TestAllocDeclAndCheckBoundary(t *testing.T) {
	const allocCap = 1000
	thread := &starlark.Thread{}
	thread.SetMaxAllocs(allocCap)

	if err := thread.CheckAllocs(allocCap); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if err := thread.CheckAllocs(allocCap + 1); err == nil {
		t.Errorf("expected error checking too-many allocations")
	}

	if err := thread.AddAllocs(allocCap); err != nil {
		t.Errorf("could not allocate entire quota: %v", err)
	} else {
		thread.AddAllocs(-allocCap)
		if err := thread.AddAllocs(allocCap + 1); err == nil {
			t.Errorf("expected error when exceeding quota")
		}
	}
}

func TestPositiveDeltaDeclaration(t *testing.T) {
	const intendedAllocIncrease = 1000

	thread := &starlark.Thread{}
	thread.SetMaxAllocs(0)

	// Accept and correctly store reasonable size increase
	allocs0 := thread.Allocs()
	if err := thread.AddAllocs(intendedAllocIncrease); err != nil {
		t.Errorf("unexpected cancellation: %v", err)
	}
	delta := thread.Allocs() - allocs0
	if delta != intendedAllocIncrease {
		t.Errorf("incorrect size increase: expected %d but got %d", intendedAllocIncrease, delta)
	}

	if _, err := starlark.ExecFile(thread, "alloc_cancel_test", "", nil); err != nil {
		t.Errorf("unexpected cancellation: %v", err)
	}
}

func TestPositiveDeltaDeclarationExceedingMax(t *testing.T) {
	const allocationIncrease = 1000
	const maxAllocs = allocationIncrease / 2

	thread := &starlark.Thread{}
	thread.SetMaxAllocs(maxAllocs)

	// Error when too much memory is required
	if err := thread.AddAllocs(allocationIncrease); err == nil {
		t.Errorf("expected allocation failure!")
	}

	if allocs := thread.Allocs(); allocs != allocationIncrease {
		t.Errorf("extra allocations were not recorded on an allocation failure: expected %d but %d were recorded", allocationIncrease, allocs)
	}

	if _, err := starlark.ExecFile(thread, "alloc_cancel_test", "", nil); err == nil {
		t.Errorf("expected cancellation")
	} else {
		expected := &starlark.AllocsSafetyError{}
		if !errors.As(err, &expected) {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestOverflowingPositiveDeltaDeclaration(t *testing.T) {
	const allocationIncrease = math.MaxInt64

	thread := &starlark.Thread{}
	thread.SetMaxAllocs(0)

	// Increase so that the next allocation will cause an overflow
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("unexpected error when declaring allocation increase: %v", err)
	}

	// Check overflow detected
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("unexpected error when overflowing allocations: %v", err)
	} else if allocs := thread.Allocs(); allocs != math.MaxUint64 {
		t.Errorf("incorrect allocations stored: expected %d but got %d", uint64(math.MaxUint64), allocs)
	}

	// Check repeated overflow
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("unexpected error when repeatedly overflowing allocations: %v", err)
	} else if allocs := thread.Allocs(); allocs != math.MaxUint64 {
		t.Errorf("incorrect allocations stored: expected %d but got %d", uint64(math.MaxUint64), allocs)
	}
}

func TestNegativeDeltaDeclaration(t *testing.T) {
	const allocGreatest = 1000
	const allocReduction = 100
	const expectedFinalAllocs = allocGreatest - allocReduction

	thread := &starlark.Thread{}
	thread.SetMaxAllocs(0)

	if err := thread.AddAllocs(allocGreatest); err != nil {
		t.Errorf("unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(-allocReduction); err != nil {
		t.Errorf("unexpected error when declaring allocation reduction: %v", err)
	}
	if allocs := thread.Allocs(); allocs != expectedFinalAllocs {
		t.Errorf("increase and reduction of allocations lead to incorrect value: expected %v but got %v", expectedFinalAllocs, allocs)
	}
}

func TestOverzealousNegativeDeltaDeclaration(t *testing.T) {
	const allocGreatest = 1000
	const allocReduction = 2 * allocGreatest
	const expectedFinalAllocs = 0

	thread := &starlark.Thread{}
	thread.SetMaxAllocs(0)

	if err := thread.AddAllocs(allocGreatest); err != nil {
		t.Errorf("unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(-allocReduction); err != nil {
		t.Errorf("unexpected error when declaring allocation reduction: %v", err)
	}
	if allocs := thread.Allocs(); allocs != 0 {
		t.Errorf("expected overzealous alloc reduction to cap allocations at zero: recorded %d allocs instead", allocs)
	}
}

func TestConcurrentCheckAllocsUsage(t *testing.T) {
	const allocPeak = math.MaxUint64 ^ (math.MaxUint64 >> 1)
	const maxAllocs = allocPeak + 1
	const repetitions = 1_000_000

	thread := &starlark.Thread{}
	thread.SetMaxAllocs(maxAllocs)
	thread.AddAllocs(allocPeak - 1)

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		// Flip between 1000...00 and 0111...11 allocations
		for i := 0; i < repetitions; i++ {
			thread.AddAllocs(1)
			thread.AddAllocs(-1)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < repetitions; i++ {
			// Check 1000...01 not exceeded
			if err := thread.CheckAllocs(1); err != nil {
				t.Errorf("unexpected error: %v", err)
				break
			}
		}
		wg.Done()
	}()

	wg.Wait()
}

func TestConcurrentAddAllocsUsage(t *testing.T) {
	const expectedAllocs = 1_000_000

	thread := &starlark.Thread{}
	thread.SetMaxAllocs(0)

	wg := sync.WaitGroup{}
	wg.Add(2)

	callAddAlloc := func(n uint) {
		for i := uint(0); i < n; i++ {
			thread.AddAllocs(1)
		}
		wg.Done()
	}

	go callAddAlloc(expectedAllocs / 2)
	go callAddAlloc(expectedAllocs / 2)

	wg.Wait()

	if allocs := thread.Allocs(); allocs != expectedAllocs {
		t.Errorf("concurrent thread.AddAlloc contains a race, expected %d allocs recorded but got %d", expectedAllocs, allocs)
	}
}

func TestCheckAllocsCancelledRejection(t *testing.T) {
	const cancellationReason = "arbitrary cancellation reason"
	const maxAllocs = 1000

	thread := &starlark.Thread{}
	thread.Cancel(cancellationReason)
	thread.SetMaxAllocs(maxAllocs)

	if err := thread.CheckAllocs(2 * maxAllocs); err == nil {
		t.Errorf("expected cancellation")
	} else if errors.Unwrap(err).Error() != cancellationReason {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddAllocsCancelledRejection(t *testing.T) {
	const cancellationReason = "arbitrary cancellation reason"
	const maxAllocs = 1000

	thread := &starlark.Thread{}
	thread.Cancel(cancellationReason)
	thread.SetMaxAllocs(maxAllocs)

	if err := thread.AddAllocs(2 * maxAllocs); err == nil {
		t.Errorf("expected cancellation")
	} else if errors.Unwrap(err).Error() != cancellationReason {
		t.Errorf("unexpected error: %v", err)
	} else if allocs := thread.Allocs(); allocs != 0 {
		t.Errorf("changes were recorded against cancelled thread: expected 0 allocations, got %v", allocs)
	}
}

func TestSafeStringBuilder(t *testing.T) {
	t.Run("over-allocation", func(t *testing.T) {
		t.Run("Grow", func(t *testing.T) {
			thread := &starlark.Thread{}
			thread.SetMaxAllocs(1)

			builder := starlark.NewSafeStringBuilder(thread)
			builder.Grow(1000)
			if err := builder.Err(); err == nil {
				t.Errorf("Grow shouldn't be able to over allocate")
			}
		})

		t.Run("Write", func(t *testing.T) {
			thread := &starlark.Thread{}
			thread.SetMaxAllocs(1)

			builder := starlark.NewSafeStringBuilder(thread)
			if _, err := builder.Write(make([]byte, 1000)); err == nil {
				t.Errorf("Write shouldn't be able to over allocate")
			}
		})

		t.Run("WriteString", func(t *testing.T) {
			thread := &starlark.Thread{}
			thread.SetMaxAllocs(1)

			builder := starlark.NewSafeStringBuilder(thread)
			if _, err := builder.WriteString("foo bar baz qux"); err == nil {
				t.Errorf("WriteString shouldn't be able to over allocate")
			}
		})

		t.Run("WriteByte", func(t *testing.T) {
			thread := &starlark.Thread{}
			thread.SetMaxAllocs(1)

			builder := starlark.NewSafeStringBuilder(thread)
			builder.Grow(4)
			if err := builder.WriteByte(1); err == nil {
				t.Errorf("WriteByte shouldn't be able to write after an over allocation attempt")
			}
		})

		t.Run("WriteRune", func(t *testing.T) {
			thread := &starlark.Thread{}
			thread.SetMaxAllocs(1)

			builder := starlark.NewSafeStringBuilder(thread)
			if _, err := builder.WriteRune(utf8.MaxRune); err == nil {
				t.Errorf("WriteRune shouldn't be able to over allocate")
			}
		})
	})

	t.Run("counting", func(t *testing.T) {
		t.Run("small", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
			st.SetMaxExecutionSteps(0)
			st.SetMaxAllocs(0)
			st.RunThread(func(thread *starlark.Thread) {
				for i := 0; i < st.N; i++ {
					builder := starlark.NewSafeStringBuilder(thread)
					st.KeepAlive(builder.String())
				}
			})
		})

		t.Run("Grow", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.RunThread(func(thread *starlark.Thread) {
				allocs := thread.Allocs()
				builder := starlark.NewSafeStringBuilder(thread)
				builder.Grow(st.N)
				if err := builder.Err(); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if uint64(builder.Cap()) != (thread.Allocs() - allocs) {
					t.Errorf("allocation size mismatch: expected %v got %v", thread.Allocs(), builder.Cap())
				}
				st.KeepAlive(builder.String())
			})
		})

		t.Run("Write", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
			st.SetMinExecutionSteps(1)
			st.SetMaxExecutionSteps(1)
			st.RunThread(func(thread *starlark.Thread) {
				allocs := thread.Allocs()
				builder := starlark.NewSafeStringBuilder(thread)
				if _, err := builder.Write(make([]byte, st.N)); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if uint64(builder.Cap()) != (thread.Allocs() - allocs) {
					t.Errorf("allocation size mismatch: expected %v got %v", thread.Allocs(), builder.Cap())
				}
				st.KeepAlive(builder.String())
			})
		})

		t.Run("WriteString", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
			st.SetMinExecutionSteps(1)
			st.SetMaxExecutionSteps(1)
			st.RunThread(func(thread *starlark.Thread) {
				allocs := thread.Allocs()
				builder := starlark.NewSafeStringBuilder(thread)
				if _, err := builder.WriteString(strings.Repeat("a", st.N)); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if uint64(builder.Cap()) != (thread.Allocs() - allocs) {
					t.Errorf("allocation size mismatch: expected %v got %v", thread.Allocs(), builder.Cap())
				}
				st.KeepAlive(builder.String())
			})
		})

		t.Run("WriteByte", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
			st.SetMinExecutionSteps(1)
			st.SetMaxExecutionSteps(1)
			st.RunThread(func(thread *starlark.Thread) {
				allocs := thread.Allocs()
				builder := starlark.NewSafeStringBuilder(thread)
				for i := 0; i < st.N; i++ {
					if err := builder.WriteByte(97); err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				}
				if uint64(builder.Cap()) != (thread.Allocs() - allocs) {
					t.Errorf("allocation size mismatch: expected %v got %v", thread.Allocs(), builder.Cap())
				}
				st.KeepAlive(builder.String())
			})
		})

		t.Run("WriteRune", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
			st.SetMinExecutionSteps(1)
			st.SetMaxExecutionSteps(1)
			st.RunThread(func(thread *starlark.Thread) {
				allocs := thread.Allocs()
				builder := starlark.NewSafeStringBuilder(thread)
				for i := 0; i < st.N; i++ {
					if _, err := builder.WriteRune('a'); err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				}
				if uint64(builder.Cap()) != (thread.Allocs() - allocs) {
					t.Errorf("allocation size mismatch: expected %v got %v", thread.Allocs(), builder.Cap())
				}
				st.KeepAlive(builder.String())
			})
		})
	})

	t.Run("allocs", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			sb := starlark.NewSafeStringBuilder(thread)
			initialAllocs := thread.Allocs()

			if _, err := sb.WriteString("foo bar baz qux"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if thread.Allocs() == initialAllocs {
				t.Error("SafeStringBuilder did not allocate")
			}

			expected := thread.Allocs() - initialAllocs
			if actual := sb.Allocs(); actual != expected {
				t.Errorf("incorrect number of allocs reported: expected %d but got %d", expected, actual)
			}
		})
	})
}
