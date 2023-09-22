package starlark_test

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

type misaligned struct {
	a int8
	c int64
}

func runEstimateTest(t *testing.T, createObj func() interface{}) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			obj := createObj()
			thread.AddAllocs(starlark.EstimateSize(obj))
			st.KeepAlive(obj)
		}
	})
}

// allocString returns a heap-allocated copy of s.
func allocString(s string) string {
	return string([]byte(s))
}

func TestEstimateBuiltinTypes(t *testing.T) {
	t.Run("int8", func(t *testing.T) {
		// In theory, this shold not allocate
		runEstimateTest(t, func() interface{} { return int8(rand.Int()) })
	})

	t.Run("*int8", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int8) })
	})

	t.Run("nil ptr", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return nil })
	})

	t.Run("int16", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return int16(rand.Int()) })
	})

	t.Run("*int16", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int16) })
	})

	t.Run("int32", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return int32(rand.Int()) })
	})

	t.Run("*int32", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int32) })
	})

	t.Run("int64", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return int64(rand.Int()) })
	})

	t.Run("*int64", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int64) })
	})

	t.Run("misaligned struct", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return misaligned{c: rand.Int63()} })
	})

	t.Run("*misaligned struct", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return &misaligned{} })
	})

	t.Run("*interface", func(t *testing.T) {
		runEstimateTest(t, func() interface{} {
			obj := interface{}(new(int))
			return &obj
		})
	})

	t.Run("empty string", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return allocString("") })
	})

	t.Run("string", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return allocString("test") })
	})
}

func TestEstimateEmptyIndirects(t *testing.T) {
	runEstimateTest(t, func() interface{} {
		return struct {
			i interface{}
			s string
			n *int
			m map[int]bool
			l []struct{}
			c chan int
		}{}
	})
}

func TestEstimateSlicePointer(t *testing.T) {
	runEstimateTest(t, func() interface{} {
		slice := make([]int, 16)

		return struct {
			ptr   *int
			slice []int
		}{
			ptr:   &slice[0],
			slice: slice,
		}
	})
}

func TestEstimateMemberPointers(t *testing.T) {
	runEstimateTest(t, func() interface{} {
		a := &struct {
			l []int
			s string
			n int

			pL *int
			pS *string
			pN *int
		}{
			s: allocString("test"),
			n: 1,
			l: make([]int, 32),
		}

		// Refer to existing memory
		a.pS = &a.s
		a.pN = &a.n
		a.pL = &a.l[0]

		return a
	})
}

func TestEstimateDuplicateIndirects(t *testing.T) {
	runEstimateTest(t, func() interface{} {
		a := struct {
			vi interface{}
			pi interface{}
			s  string
			n  *int
			m  map[int]bool
			l  []struct{}
			c  chan int
		}{
			s: allocString("test"),
			n: new(int),
			m: make(map[int]bool, 16),
			l: make([]struct{}, 0, 16),
			c: make(chan int, 16),
		}

		// Make a loop for the interface
		a.vi = a
		a.pi = &a

		return []interface{}{a, a}
	})
}

func TestNil(t *testing.T) {
	if starlark.EstimateSize(nil) != 0 {
		t.Errorf("estimateSize for nil must be 0")
	}

	var nilMap map[int]int
	if starlark.EstimateSize(nilMap) != 0 {
		t.Errorf("EstimateSize for nil must be 0")
	}

	var nilPtrMap *map[int]int
	if starlark.EstimateSize(nilPtrMap) != 0 {
		t.Errorf("EstimateSize for nil must be 0")
	}

	var nilChan chan int
	if starlark.EstimateSize(nilChan) != 0 {
		t.Errorf("EstimateSize for nil must be 0")
	}

	var nilPtrChan *chan int
	if starlark.EstimateSize(nilPtrChan) != 0 {
		t.Errorf("EstimateSize for nil must be 0")
	}

	var nilSlice *[]int
	if starlark.EstimateSize(nilSlice) != 0 {
		t.Errorf("EstimateSize for nil must be 0")
	}

	var nilPtr *int
	if starlark.EstimateSize(nilPtr) != 0 {
		t.Errorf("EstimateSize for nil must be 0")
	}

	var emptyStruct struct {
		a map[int]int
		b *map[int]int
		c chan int
		d *chan int
		e *int
		s *[]int
	}

	if s := starlark.EstimateSize(emptyStruct); s != int64(unsafe.Sizeof(emptyStruct)) {
		t.Errorf("expected size %d got %d", unsafe.Sizeof(emptyStruct), s)
	}
}

func TestEstimateMap(t *testing.T) {
	t.Run("map[int]int", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			value := make(map[int]int)

			for i := 0; i < st.N; i++ {
				value[i] = i
			}

			thread.AddAllocs(starlark.EstimateSize(value))
			st.KeepAlive(value)
		})
	})

	t.Run("map[int]<big-struct>", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			value := make(map[int][64]int)

			for i := 0; i < st.N; i++ {
				value[i] = [64]int{}
			}

			thread.AddAllocs(starlark.EstimateSize(value))
			st.KeepAlive(value)
		})
	})

	t.Run("existing map[any]bool", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			dict := make(map[interface{}]struct{})

			for i := 0; i < st.N; i++ {
				value := interface{}(i)
				dict[value] = struct{}{}
				// I count here directly the value, so that I
				// can use `EstimateSize` instead of `EstimateSizeDeep`
				thread.AddAllocs(starlark.EstimateSize(value))
				st.KeepAlive(value)
			}

			thread.AddAllocs(starlark.EstimateSize(dict))
			st.KeepAlive(dict)
		})
	})
}

func TestEstimateChan(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			value := make(chan int, st.N)

			for i := 0; i < st.N; i++ {
				value <- i
			}

			thread.AddAllocs(starlark.EstimateSize(value))
			st.KeepAlive(value)
		})
	})
}

func TestTinyAllocator(t *testing.T) {
	var anchor *int64
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			anchor = new(int64)
			*anchor = int64(i)

			value := int64(i) * 0xcc9e2d51

			thread.AddAllocs(starlark.EstimateSize(value))
			st.KeepAlive(value)
		}
	})
}

func TestEstimateString(t *testing.T) {
	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				value := string([]byte("Hello World!"))

				thread.AddAllocs(starlark.EstimateSize(value))
				st.KeepAlive(value)
			}
		})
	})

	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			value := strings.Repeat("Hello World!", st.N)

			thread.AddAllocs(starlark.EstimateSize(value))
			st.KeepAlive(value)
		})
	})
}

func TestEstimateMakeSize(t *testing.T) {
	t.Run("slice", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			t.Run("with-len", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize([]starlark.Value{}, st.N)); err != nil {
						st.Error(err)
					}
					st.KeepAlive(make([]starlark.Value, st.N))
				})
			})

			t.Run("with-cap", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize([]starlark.Value{}, st.N)); err != nil {
						st.Error(err)
					}
					st.KeepAlive(make([]starlark.Value, 0, st.N))
				})
			})

			t.Run("complex-elems", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize([][2]starlark.Value{}, st.N)); err != nil {
						st.Error(err)
					}
					st.KeepAlive(make([][2]starlark.Value, st.N))
				})
			})
		})

		t.Run("single", func(t *testing.T) {
			t.Run("small-elem", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					const str = "foo"
					if err := thread.AddAllocs(starlark.EstimateMakeSize([]string{str}, st.N)); err != nil {
						st.Error(err)
					}

					ret := make([]string, st.N)
					for i := 0; i < len(ret); i++ {
						ret[i] = string([]byte(str))
					}

					st.KeepAlive(ret)
				})
			})

			t.Run("large-elem", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize([]map[int][64]int{{}}, st.N)); err != nil {
						st.Error(err)
					}

					val := make([]map[int][64]int, st.N)
					for i := 0; i < len(val); i++ {
						val[i] = map[int][64]int{}
					}

					st.KeepAlive(val)
				})
			})
		})
	})

	t.Run("map", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			t.Run("small-elems", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize(map[int]int{}, st.N)); err != nil {
						st.Error(err)
					}

					st.KeepAlive(make(map[int]int, st.N))
				})
			})

			t.Run("large-elems", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize(map[[16]int64][256]byte{}, st.N)); err != nil {
						st.Error(err)
					}

					st.KeepAlive(make(map[[16]int][256]byte, st.N))
				})
			})
		})

		t.Run("single", func(t *testing.T) {
			t.Run("string-keys", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize(map[string]int{"kxxxxxxxx": 1}, st.N)); err != nil {
						st.Error(err)
					}

					ret := make(map[string]int, st.N)
					for i := 0; i < len(ret); i++ {
						key := fmt.Sprintf("k%8d", i)
						ret[key] = i
					}

					st.KeepAlive(ret)
				})
			})

			t.Run("slice-values", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize(map[int][]int{0: {0}}, st.N)); err != nil {
						st.Error(err)
					}

					val := make(map[int][]int, st.N)
					for i := 0; i < len(val); i++ {
						val[i] = []int{i}
					}

					st.KeepAlive(val)
				})
			})
		})
	})

	t.Run("chan", func(t *testing.T) {
		t.Run("unbuffered", func(t *testing.T) {
			t.Run("empty", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					for i := 0; i < st.N; i++ {
						var channel chan int
						if err := thread.AddAllocs(starlark.EstimateMakeSize(channel, st.N)); err != nil {
							st.Error(err)
						}
						st.KeepAlive(channel)
					}
				})
			})
		})

		t.Run("buffered", func(t *testing.T) {
			t.Run("empty", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize(make(chan int, 4*st.N), st.N)); err != nil {
						st.Error(err)
					}
					st.KeepAlive(make(chan int, st.N))
				})
			})

			t.Run("populated", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.MemSafe)
				st.RunThread(func(thread *starlark.Thread) {
					if err := thread.AddAllocs(starlark.EstimateMakeSize(make(chan int), st.N)); err != nil {
						st.Error(err)
					}

					channel := make(chan int, st.N)
					for i := 0; i < st.N; i++ {
						channel <- i
					}
					st.KeepAlive(channel)
				})
			})
		})
	})

	t.Run("invalid", func(t *testing.T) {
		tests := []struct {
			name     string
			expect   string
			template interface{}
		}{{
			name:     "invalid-type",
			expect:   "template must be a slice, map or chan: got float64",
			template: 0.0,
		}, {
			name:     "slice",
			expect:   "template length must be at most 1: got length 2",
			template: []string{"spanner", "wrench"},
		}, {
			name:   "map",
			expect: "template length must be at most 1: got length 2",
			template: map[string]float64{
				"pi":  math.Pi,
				"phi": math.Phi,
			},
		}, {
			name:   "chan",
			expect: "template length must be at most 1: got length 3",
			template: func() interface{} {
				ret := make(chan int, 3)
				ret <- 1
				ret <- 2
				ret <- 3
				return ret
			}(),
		}}

		catch := func(fn func()) (v interface{}, panicked bool) {
			defer func() {
				if v = recover(); v != nil {
					panicked = true
				}
			}()
			fn()
			return
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				err, panicked := catch(func() {
					starlark.EstimateMakeSize(test.template, 25)
				})
				if !panicked {
					t.Error("invalid MakeSizeInput did not cause panic")
				}
				if err != test.expect {
					t.Errorf("unexpected error: %v", err)
				}
			})
		}
	})
}

func TestSizeConstants(t *testing.T) {
	constantTest := func(t *testing.T, constant int64, value func() interface{}) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				if err := thread.AddAllocs(constant); err != nil {
					st.Error(err)
				}
				st.KeepAlive(value())
			}
		})
	}

	t.Run("string", func(t *testing.T) {
		constantTest(t, starlark.StringTypeOverhead, func() interface{} { return reflect.StringHeader{} })
	})
	t.Run("slice", func(t *testing.T) {
		constantTest(t, starlark.SliceTypeOverhead, func() interface{} { return make([]struct{}, 0, 0) })
		constantTest(t, starlark.SliceTypeOverhead, func() interface{} { return make([][256]byte, 0, 0) })
	})
}
