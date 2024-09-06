package starlark_test

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

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
		defer checkPanic("NewSafeAppender: expected pointer to slice, got nil")
		thread := &starlark.Thread{}
		starlark.NewSafeAppender(thread, nil)
	})

	t.Run("non-pointer", func(t *testing.T) {
		defer checkPanic("NewSafeAppender: expected pointer to slice, got int")
		thread := &starlark.Thread{}
		starlark.NewSafeAppender(thread, 25)
	})

	t.Run("non-slice pointer", func(t *testing.T) {
		defer checkPanic("NewSafeAppender: expected pointer to slice, got pointer to int")
		thread := &starlark.Thread{}
		starlark.NewSafeAppender(thread, new(int))
	})
}

func TestSafeAppenderAppend(t *testing.T) {
	t.Run("ints", func(t *testing.T) {
		t.Run("no-allocation", func(t *testing.T) {
			storage := make([]int, 0, 16)
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMaxAllocs(0)
			st.SetMinSteps(1)
			st.SetMaxSteps(1)
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
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMinSteps(2)
			st.SetMaxSteps(2)
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
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMinSteps(1)
			st.SetMaxSteps(1)
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
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(1)
		st.SetMaxSteps(1)
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
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMinSteps(3)
			st.SetMaxSteps(3)
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
			st.RequireSafety(starlark.MemSafe)
			st.SetMinSteps(1)
			st.SetMaxSteps(1)
			st.RunThread(func(thread *starlark.Thread) {
				if err := thread.AddAllocs(starlark.SafeMul64(starlark.EstimateSize(0), int64(st.N))); err != nil {
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
		const expected = "SafeAppender.AppendSlice: expected slice, got nil"
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

	t.Run("non-slice", func(t *testing.T) {
		const expected = "SafeAppender.AppendSlice: expected slice, got string"
		defer func() {
			if err := recover(); err != nil {
				if err != expected {
					t.Errorf("unexpected panic expected %#v but got %#v", expected, err)
				}
			} else {
				t.Error("expected panic")
			}
		}()

		nonslice := []int{}
		sa := starlark.NewSafeAppender(&starlark.Thread{}, &nonslice)
		sa.AppendSlice("spanner")
	})

	t.Run("ints", func(t *testing.T) {
		t.Run("no-allocation", func(t *testing.T) {
			storage := make([]int, 0, 16)
			toAppend := []int{1, 2, 3, 4}
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMaxAllocs(0)
			st.SetMinSteps(int64(len(toAppend)))
			st.SetMaxSteps(int64(len(toAppend)))
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
			toAppend := []int{-1, -1}
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMinSteps(int64(len(toAppend)))
			st.SetMaxSteps(int64(len(toAppend)))
			st.RunThread(func(thread *starlark.Thread) {
				for i := 0; i < st.N; i++ {
					slice := []int{1, 3, 5}
					if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
						st.Error(err)
					}
					sa := starlark.NewSafeAppender(thread, &slice)
					if err := sa.AppendSlice(toAppend); err != nil {
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
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMinSteps(1)
			st.SetMaxSteps(1)
			st.RunThread(func(thread *starlark.Thread) {
				slice := []int{1, 3, 5}
				if err := thread.AddAllocs(starlark.SafeAdd64(starlark.EstimateSize(slice), starlark.SliceTypeOverhead)); err != nil {
					st.Error(err)
				}
				st.KeepAlive(slice)

				toAppend := make([]int, 0, st.N)
				expected := slice
				for i := 0; i < st.N; i++ {
					toAppend = append(toAppend, -i)
					expected = append(expected, -i)
				}

				sa := starlark.NewSafeAppender(thread, &slice)
				if err := sa.AppendSlice(toAppend); err != nil {
					st.Error(err)
				}
				if err := thread.AddAllocs(starlark.SliceTypeOverhead); err != nil {
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
		// 	t.Run("many-small", func(t *testing.T) {
		// 		toAppend := []interface{}{0.0, rune(0)}
		// 		st := startest.From(t)
		// 		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		// 		st.SetMinSteps(int64(len(toAppend)))
		// 		st.SetMaxSteps(int64(len(toAppend)))
		// 		st.RunThread(func(thread *starlark.Thread) {
		// 			for i := 0; i < st.N; i++ {
		// 				slice := []interface{}{false, 0, ""}
		// 				if err := thread.AddAllocs(starlark.EstimateSize(slice)); err != nil {
		// 					st.Error(err)
		// 				}
		// 				sa := starlark.NewSafeAppender(thread, &slice)
		// 				if err := sa.AppendSlice(toAppend); err != nil {
		// 					st.Error(err)
		// 				}
		// 				st.KeepAlive(slice)

		// 				expected := []interface{}{false, 0, "", 0.0, rune(0)}
		// 				if !reflect.DeepEqual(slice, expected) {
		// 					t.Errorf("expected %v, got %v", expected, slice)
		// 				}
		// 			}
		// 		})
		// 	})

		t.Run("one-large", func(t *testing.T) {
			initialSlice := []interface{}{false, false}
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
			st.SetMinSteps(1)
			st.SetMaxSteps(1)
			st.RunThread(func(thread *starlark.Thread) {
				if err := thread.AddAllocs(starlark.SafeMul64(starlark.EstimateSize(0), int64(st.N))); err != nil {
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
	assertPanics := func(t *testing.T) {
		if recover() == nil {
			t.Error("expected panic")
		}
	}

	t.Run("Append", func(t *testing.T) {
		t.Run("incompatible-kinds", func(t *testing.T) {
			defer assertPanics(t)
			thread := &starlark.Thread{}
			slice := []int{1, 3, 5}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.Append("nope")
		})

		t.Run("unimplemented-interface", func(t *testing.T) {
			defer assertPanics(t)
			thread := &starlark.Thread{}
			slice := []starlark.Value{starlark.False, starlark.MakeInt(0)}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.Append("nope")
		})

		t.Run("incompatible-interfaces", func(t *testing.T) {
			defer assertPanics(t)
			thread := &starlark.Thread{}
			slice := []interface{ private() }{}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.Append("nope")
		})
	})

	t.Run("AppendSlice", func(t *testing.T) {
		t.Run("incompatible-kinds", func(t *testing.T) {
			defer assertPanics(t)
			thread := &starlark.Thread{}
			slice := []int{}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.AppendSlice([]interface{}{})
		})

		t.Run("incompatible-interfaces", func(t *testing.T) {
			defer assertPanics(t)
			thread := &starlark.Thread{}
			slice := []interface{ private() }{}
			sa := starlark.NewSafeAppender(thread, &slice)
			sa.AppendSlice([]interface{}{})
		})
	})
}

func TestSafeAppenderErrorReturn(t *testing.T) {
	thread := &starlark.Thread{}
	thread.SetMaxAllocs(100)
	var slice []int
	sa := starlark.NewSafeAppender(thread, &slice)

	for i := 0; i < 10000; i++ {
		if err := sa.Append(1); err != nil {
			if !errors.Is(err, starlark.ErrSafety) {
				t.Errorf("unexpected error: %v", err)
			}
			return
		}
	}
	t.Error("expected error")
}

func TestSafeAppenderNil(t *testing.T) {
	tests := []struct {
		name   string
		slice  interface{}
		expect string
	}{{
		name:  "interface",
		slice: &[]interface{}{},
	}, {
		name:  "pointer",
		slice: &[]*struct{}{},
	}, {
		name:  "slice",
		slice: &[][]int{},
	}, {
		name:  "map",
		slice: &[]map[int]int{},
	}, {
		name:  "chan",
		slice: &[]chan int{},
	}, {
		name:   "bool",
		slice:  &[]bool{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "int",
		slice:  &[]int{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "int8",
		slice:  &[]int8{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "int16",
		slice:  &[]int16{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "int32",
		slice:  &[]int32{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "int64",
		slice:  &[]int64{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "uint",
		slice:  &[]uint{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "uint8",
		slice:  &[]uint8{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "uint16",
		slice:  &[]uint16{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "uint32",
		slice:  &[]uint32{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "uint64",
		slice:  &[]uint64{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "rune",
		slice:  &[]rune{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "uintptr",
		slice:  &[]uintptr{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "float32",
		slice:  &[]float32{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "float64",
		slice:  &[]float64{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "complex64",
		slice:  &[]complex64{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "complex128",
		slice:  &[]complex128{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "string",
		slice:  &[]string{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "array",
		slice:  &[][10]int{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "struct",
		slice:  &[]struct{}{},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "func",
		slice:  &[]func(){},
		expect: "SafeAppender.Append: unexpected nil",
	}, {
		name:   "unsafe.Pointer",
		slice:  &[]unsafe.Pointer{},
		expect: "SafeAppender.Append: unexpected nil",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				msg := recover()
				if msg == nil && test.expect != "" {
					t.Error("expected panic")
				} else if msg != nil && msg != test.expect {
					t.Errorf("expected panic %#v but got: %v", test.expect, msg)
				}
			}()
			sa := starlark.NewSafeAppender(&starlark.Thread{}, test.slice)
			if err := sa.Append(nil); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSafeAppenderAllocCounting(t *testing.T) {
	t.Run("Append", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			if err := thread.AddAllocs(100); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			initial := thread.Allocs()

			slice := []byte{}
			sa := starlark.NewSafeAppender(thread, &slice)
			for i := 0; i < st.N; i++ {
				if err := sa.Append(byte(1)); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			expected := thread.Allocs() - initial
			if actual := sa.Allocs(); actual != expected {
				t.Errorf("incorrect number of allocations reported: expected %d but got %d", expected, actual)
			}
		})
	})

	t.Run("AppendSlice", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			if err := thread.AddAllocs(100); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			initial := thread.Allocs()

			slice := []byte{}
			sa := starlark.NewSafeAppender(thread, &slice)
			toAppend := []byte{}
			for i := 0; i < st.N; i++ {
				toAppend = append(toAppend, byte(i))
			}
			if err := sa.AppendSlice(toAppend); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expected := thread.Allocs() - initial
			if actual := sa.Allocs(); actual != expected {
				t.Errorf("incorrect number of allocations reported: expected %d but got %d", expected, actual)
			}
		})
	})
}
