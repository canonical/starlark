package starlark_test

import (
	"reflect"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestSafeAppenderInputValidation(t *testing.T) {
	checkPanic := func(expected string) {
		if err := recover(); err != nil {
			if err != expected {
				t.Errorf("unexpected panic expected %#v but got %#v", expected, err)
			}
		} else {
			t.Error("expected panic")
		}
	}

	t.Run("nil", func(t *testing.T) {
		defer checkPanic("expected pointer to slice, got nil")
		thread := &starlark.Thread{}
		starlark.NewSafeAppender(thread, nil)
	})

	t.Run("non-pointer", func(t *testing.T) {
		defer checkPanic("expected pointer to slice, got int")
		thread := &starlark.Thread{}
		starlark.NewSafeAppender(thread, 25)
	})

	t.Run("non-slice pointer", func(t *testing.T) {
		defer checkPanic("expected pointer to slice, got pointer to int")
		thread := &starlark.Thread{}
		starlark.NewSafeAppender(thread, new(int))
	})
}

func TestSafeAppenderAppend(t *testing.T) {
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
			st.RunThread(func(thread *starlark.Thread) {
				slice := []int{1, 3, 5}
				if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
					st.Error(err)
				}
				expected := slice
				var toAppend []interface{}
				for i := 0; i < st.N; i++ {
					toAppend = append(toAppend, -i)
					expected = append(expected, -i)
				}

				sa := starlark.NewSafeAppender(thread, &slice)
				if err := sa.Append(toAppend...); err != nil {
					st.Error(err)
				}
				st.KeepAlive(slice)

				if !reflect.DeepEqual(slice, expected) {
					t.Errorf("expected %v, got %v", expected, slice)
				}
			})
		})
	})

	t.Run("big-struct", func(t *testing.T) {
		st := startest.From(t)
		st.RunThread(func(thread *starlark.Thread) {
			slice := [][100]int{}
			appender := starlark.NewSafeAppender(thread, &slice)
			for i := 0; i < st.N; i++ {
				appender.Append([100]int{})
			}
			if len(slice) != st.N {
				t.Errorf("expected %d elements, got %d", st.N, len(slice))
			}
			st.KeepAlive(slice)
		})
	})

	t.Run("interfaces", func(t *testing.T) {
		t.Run("many-small", func(t *testing.T) {
			st := startest.From(t)
			st.RunThread(func(thread *starlark.Thread) {
				for i := 0; i < st.N; i++ {
					slice := []interface{}{false, 0, ""}
					if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
						st.Error(err)
					}
					sa := starlark.NewSafeAppender(thread, &slice)
					if err := sa.Append(0.0, rune(0), nil); err != nil {
						st.Error(err)
					}
					st.KeepAlive(slice)

					expected := []interface{}{false, 0, "", 0.0, rune(0), nil}
					if !reflect.DeepEqual(slice, expected) {
						t.Errorf("expected %v, got %v", expected, slice)
					}
				}
			})
		})

		t.Run("one-large", func(t *testing.T) {
			initialSlice := []interface{}{false, false}
			st := startest.From(t)
			st.RunThread(func(thread *starlark.Thread) {
				if err := thread.AddAllocs(starlark.EstimateSize(0) * int64(st.N)); err != nil {
					st.Error(err)
				}
				toAppend := make([]interface{}, st.N)
				for i := 0; i < st.N; i++ {
					toAppend[i] = -i
				}
				expected := append(initialSlice, toAppend...)

				slice := initialSlice
				if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
					st.Error(err)
				}

				sa := starlark.NewSafeAppender(thread, &slice)
				if err := sa.Append(toAppend...); err != nil {
					st.Error(err)
				}
				st.KeepAlive(slice)

				if !reflect.DeepEqual(slice, expected) {
					t.Errorf("expected %v, got %v", expected, slice)
				}
			})
		})
	})
}

func TestSafeAppenderAppendSlice(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		const expected = "expected slice, got nil"
		defer func() {
			if err := recover(); err != nil {
				if err != expected {
					t.Errorf("unexpected panic expected %#v but got %#v", expected, err)
				}
			} else {
				t.Error("expected panic")
			}
		}()

		slice := []int{}
		sa := starlark.NewSafeAppender(&starlark.Thread{}, &slice)
		sa.AppendSlice(nil)
	})
	t.Run("ints", func(t *testing.T) {
		t.Run("no-allocation", func(t *testing.T) {
			storage := make([]int, 0, 16)
			toAppend := []int{1, 2, 3, 4}
			st := startest.From(t)
			st.SetMaxAllocs(0)
			st.RunThread(func(thread *starlark.Thread) {
				appender := starlark.NewSafeAppender(thread, &storage)
				for i := 0; i < st.N; i++ {
					appender.AppendSlice(toAppend)
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
					if err := sa.AppendSlice([]int{-1, -1}); err != nil {
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
			st.RunThread(func(thread *starlark.Thread) {
				slice := []int{1, 3, 5}
				expected := slice
				var toAppend []int
				for i := 0; i < st.N; i++ {
					toAppend = append(toAppend, -i)
					expected = append(expected, -i)
				}

				sa := starlark.NewSafeAppender(thread, &slice)
				if err := sa.AppendSlice(toAppend); err != nil {
					st.Error(err)
				}
				st.KeepAlive(slice)

				if !reflect.DeepEqual(slice, expected) {
					t.Errorf("expected %v, got %v", expected, slice)
				}
			})
		})
	})

	t.Run("interfaces", func(t *testing.T) {
		t.Run("many-small", func(t *testing.T) {
			st := startest.From(t)
			st.RunThread(func(thread *starlark.Thread) {
				for i := 0; i < st.N; i++ {
					slice := []interface{}{false, 0, ""}
					if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
						st.Error(err)
					}
					sa := starlark.NewSafeAppender(thread, &slice)
					if err := sa.AppendSlice([]interface{}{0.0, rune(0)}); err != nil {
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
			initialSlice := []interface{}{false, false}
			st := startest.From(t)
			st.RunThread(func(thread *starlark.Thread) {
				if err := thread.AddAllocs(starlark.EstimateSize(0) * int64(st.N)); err != nil {
					st.Error(err)
				}
				toAppend := make([]interface{}, st.N)
				for i := 0; i < st.N; i++ {
					toAppend[i] = -i
				}
				expected := append(initialSlice, toAppend...)

				slice := initialSlice
				if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
					st.Error(err)
				}

				sa := starlark.NewSafeAppender(thread, &slice)
				if err := sa.AppendSlice(toAppend); err != nil {
					st.Error(err)
				}
				st.KeepAlive(slice)

				if !reflect.DeepEqual(slice, expected) {
					t.Errorf("expected %v, got %v", expected, slice)
				}
			})
		})
	})
}

func TestSafeAppenderAppendTypeMismatch(t *testing.T) {
	checkPanic := func(t *testing.T) {
		if recover() == nil {
			t.Error("expected panic")
		}
	}

	t.Run("Append", func(t *testing.T) {
		t.Run("incompatible-kinds", func(t *testing.T) {
			defer checkPanic(t)
			thread := &starlark.Thread{}
			slice := []int{1, 3, 5}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.Append("nope")
		})

		t.Run("unimplemented-interface", func(t *testing.T) {
			defer checkPanic(t)
			thread := &starlark.Thread{}
			slice := []starlark.Value{starlark.False, starlark.MakeInt(0)}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.Append("nope")
		})

		t.Run("incompatible-interfaces", func(t *testing.T) {
			defer checkPanic(t)
			thread := &starlark.Thread{}
			slice := []interface{ private() }{}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.Append("nope")
		})
	})

	t.Run("AppendSlice", func(t *testing.T) {
		t.Run("incompatible-kinds", func(t *testing.T) {
			defer checkPanic(t)
			thread := &starlark.Thread{}
			slice := []int{}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.AppendSlice([]interface{}{})
		})

		t.Run("incompatible-interfaces", func(t *testing.T) {
			defer checkPanic(t)
			thread := &starlark.Thread{}
			slice := []interface{ private() }{}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.AppendSlice([]interface{}{})
		})
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
