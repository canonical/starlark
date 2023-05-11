package starlark_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func panicValue(t *testing.T, f func()) (msg interface{}, panicked bool) {
	defer func() {
		if msg = recover(); msg != nil {
			panicked = true
		}
	}()

	f()

	return
}

func TestSafeAppenderInputValidation(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		const expected = "expected pointer to slice, got nil"

		thread := &starlark.Thread{}
		val, panicked := panicValue(t, func() {
			starlark.NewSafeAppender(thread, nil)
		})
		if !panicked {
			t.Error("expected panic")
		}
		if val != expected {
			t.Errorf("unexpected panic expected %#v but got %#v", expected, val)
		}
	})

	t.Run("non-pointer", func(t *testing.T) {
		const expected = "expected pointer to slice, got int"

		thread := &starlark.Thread{}
		val, panicked := panicValue(t, func() {
			starlark.NewSafeAppender(thread, 25)
		})
		if !panicked {
			t.Error("expected panic")
		}
		if val != expected {
			t.Errorf("unexpected panic expected %#v but got %#v", expected, val)
		}
	})

	t.Run("non-slice pointer", func(t *testing.T) {
		const expected = "expected pointer to slice, got pointer to int"

		thread := &starlark.Thread{}
		val, panicked := panicValue(t, func() {
			starlark.NewSafeAppender(thread, new(int))
		})
		if !panicked {
			t.Error("expected panic")
		}
		if val != expected {
			t.Errorf("unexpected panic expected %#v but got %#v", expected, val)
		}
	})
}

func TestSafeAppender(t *testing.T) {
	t.Run("ints", func(t *testing.T) {
		t.Run("no-allocation", func(t *testing.T) {
			storage := make([]int, 0, 16)
			st := startest.From(t)
			st.SetMaxAllocs(0)
			st.RunThread(func(thread *starlark.Thread) {
				appender := starlark.NewSafeAppender(thread, &storage)
				for i := 0; i < st.N; i++ {
					appender.Append(i)
					if len(storage) == cap(storage) {
						storage = storage[:0]
					}
				}
				st.KeepAlive(storage)
			})
		})

		t.Run("many-small", func(t *testing.T) {
			st := startest.From(t)
			st.RunThread(func(thread *starlark.Thread) {
				for i := 0; i < st.N; i++ {
					slice := []int{1, 3, 5}
					if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
						st.Error(err)
					}
					sa := starlark.NewSafeAppender(thread, &slice)
					if err := sa.Append(-1, -1); err != nil {
						st.Error(err)
					}
					st.KeepAlive(slice)

					expected := []int{1, 3, 5, -1, -1}
					if !reflect.DeepEqual(slice, expected) {
						t.Errorf("expected %v, got %v", expected, slice)
					}
				}
			})
		})

		t.Run("one-large", func(t *testing.T) {
			st := startest.From(t)

			// st.SetMaxAllocs(0) // TODO(kcza): test allocations

			st.RunThread(func(thread *starlark.Thread) {
				slice := []int{1, 3, 5}
				if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
					st.Error(err)
				}
				var toAppend []interface{}
				var expectAppended []int
				for i := 0; i < st.N; i++ {
					toAppend = append(toAppend, -i)
					expectAppended = append(expectAppended, -i)
				}

				sa := starlark.NewSafeAppender(thread, &slice)
				if err := sa.Append(toAppend...); err != nil {
					st.Error(err)
				}
				st.KeepAlive(slice)

				expected := append([]int{1, 3, 5}, expectAppended...)
				if !reflect.DeepEqual(slice, expected) {
					t.Errorf("expected %v, got %v", expected, slice)
				}
			})
		})
	})

	t.Run("interfaces", func(t *testing.T) {
		t.Run("many-small", func(t *testing.T) {
			st := startest.From(t)

			// st.SetMaxAllocs(0) // TODO(kcza): test allocations

			st.RunThread(func(thread *starlark.Thread) {
				for i := 0; i < st.N; i++ {
					slice := []interface{}{false, 0, ""}
					if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
						st.Error(err)
					}
					sa := starlark.NewSafeAppender(thread, &slice)
					if err := sa.Append(0.0, rune(0)); err != nil {
						st.Error(err)
					}
					st.KeepAlive(slice)

					expected := []interface{}{false, 0, "", 0.0, rune(0)}
					if !reflect.DeepEqual(slice, expected) {
						t.Errorf("expected %v, got %v", expected, slice)
					}
				}
			})
		})

		t.Run("one-large", func(t *testing.T) {
			st := startest.From(t)

			// st.SetMaxAllocs(0) // TODO(kcza): test allocations

			st.RunThread(func(thread *starlark.Thread) {
				slice := []interface{}{false, false}
				if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
					st.Error(err)
				}
				var toAppend []interface{}
				for i := 0; i < st.N; i++ {
					toAppend = append(toAppend, -i)
				}

				sa := starlark.NewSafeAppender(thread, &slice)
				if err := sa.Append(toAppend...); err != nil {
					st.Error(err)
				}
				st.KeepAlive(slice)

				expected := append([]interface{}{false, false}, toAppend...)
				if !reflect.DeepEqual(slice, expected) {
					t.Errorf("expected %v, got %v", expected, slice)
				}
			})
		})
	})
}

func TestSafeAppenderTypeMismatch(t *testing.T) {
	t.Run("incompatible-kinds", func(t *testing.T) {
		thread := &starlark.Thread{}
		slice := []int{1, 3, 5}
		sa := starlark.NewSafeAppender(thread, &slice)

		_, panicked := panicValue(t, func() { sa.Append("nope") })
		if !panicked {
			t.Error("expected panic")
		}
	})

	t.Run("unimplemented-interface", func(t *testing.T) {
		thread := &starlark.Thread{}
		slice := []starlark.Value{starlark.False, starlark.MakeInt(0)}
		sa := starlark.NewSafeAppender(thread, &slice)

		_, panicked := panicValue(t, func() { sa.Append("nope") })
		if !panicked {
			t.Error("expected panic")
		}
	})

	t.Run("incompatible-interfaces", func(t *testing.T) {
		var a fmt.Stringer
		if _, ok := a.(error); ok {
			t.Fatal("interface conversion succeeded (test requires failure)")
		}

		thread := &starlark.Thread{}
		slice := []fmt.Stringer{starlark.False, starlark.Float(0.0)}
		sa := starlark.NewSafeAppender(thread, &slice)

		_, panicked := panicValue(t, func() { sa.Append(errors.New("nope")) })
		if !panicked {
			t.Error("expected panic")
		}
	})
}

func TestSafeAppenderErrorReturn(t *testing.T) {
	const expected = "exceeded memory allocation limits"

	thread := &starlark.Thread{}
	thread.SetMaxAllocs(100)
	var slice []int
	sa := starlark.NewSafeAppender(thread, &slice)

	for i := 0; i < 10000; i++ {
		if err := sa.Append(1); err != nil {
			if msg := err.Error(); msg != expected {
				t.Errorf("unexpected error: %v", msg)
			}
			return
		}
	}
	t.Error("expected error")
}
