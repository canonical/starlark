package starlark_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

type misaligned struct {
	a int8
	c int64
}

func runEstimateTest(t *testing.T, createObj func() interface{}) {
	st := startest.From(t)

	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			obj := createObj()
			thread.AddAllocs(int64(starlark.EstimateSize(obj)))
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

func TestNilInterface(t *testing.T) {
	if starlark.EstimateSize(nil) != 0 {
		t.Errorf("EstimateSize for nil must be 0")
	}
}

func TestEstimateMap(t *testing.T) {
	t.Run("map[int]int", func(t *testing.T) {
		st := startest.From(t)
		st.RunThread(func(thread *starlark.Thread) {
			value := make(map[int]int)

			for i := 0; i < st.N; i++ {
				value[i] = i
			}

			thread.AddAllocs(int64(starlark.EstimateSize(value)))
			st.KeepAlive(value)
		})
	})

	t.Run("map[int]<big-struct>", func(t *testing.T) {
		st := startest.From(t)
		st.RunThread(func(thread *starlark.Thread) {
			value := make(map[int][64]int)

			for i := 0; i < st.N; i++ {
				value[i] = [64]int{}
			}

			thread.AddAllocs(int64(starlark.EstimateSize(value)))
			st.KeepAlive(value)
		})
	})

	t.Run("existing map[any]bool", func(t *testing.T) {
		st := startest.From(t)

		st.RunThread(func(thread *starlark.Thread) {
			dict := make(map[interface{}]struct{})

			for i := 0; i < st.N; i++ {
				value := interface{}(i)
				dict[value] = struct{}{}
				// I count here directly the value, so that I
				// can use `EstimateSize` instead of `EstimateSizeDeep`
				thread.AddAllocs(int64(starlark.EstimateSize(value)))
				st.KeepAlive(value)
			}

			thread.AddAllocs(int64(starlark.EstimateSize(dict)))
			st.KeepAlive(dict)
		})
	})
}

func TestEstimateChan(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		st := startest.From(t)
		st.RunThread(func(thread *starlark.Thread) {
			value := make(chan int, st.N)

			for i := 0; i < st.N; i++ {
				value <- i
			}

			thread.AddAllocs(int64(starlark.EstimateSize(value)))
			st.KeepAlive(value)
		})
	})
}

func TestTinyAllocator(t *testing.T) {
	var anchor *int64
	st := startest.From(t)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			anchor = new(int64)
			*anchor = int64(i)

			value := int64(i) * 0xcc9e2d51

			thread.AddAllocs(int64(starlark.EstimateSize(value)))
			st.KeepAlive(value)
		}
	})
}

func TestEstimateString(t *testing.T) {
	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				value := string([]byte("Hello World!"))

				thread.AddAllocs(int64(starlark.EstimateSize(value)))
				st.KeepAlive(value)
			}
		})
	})

	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RunThread(func(thread *starlark.Thread) {
			value := strings.Repeat("Hello World!", st.N)

			thread.AddAllocs(int64(starlark.EstimateSize(value)))
			st.KeepAlive(value)
		})
	})
}
