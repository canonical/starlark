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
			thread.AddAllocs(int64(starlark.EstimateSizeDeep(obj)))
			st.KeepAlive(obj)
		}
	})
}

func TestEstimateBuiltinTypes(t *testing.T) {
	t.Run("int8", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return int8(rand.Int()) })
	})

	t.Run("*int8", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int8) })
	})

	t.Run("*int8", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return nil })
	})

	t.Run("uint8", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return uint8(rand.Int()) })
	})

	t.Run("*uint8", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(uint8) })
	})

	t.Run("int16", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return int16(rand.Int()) })
	})

	t.Run("*int16", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int16) })
	})

	t.Run("uint16", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return uint16(rand.Int()) })
	})

	t.Run("*uint16", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(uint16) })
	})

	t.Run("int32", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return int32(rand.Int()) })
	})

	t.Run("*int32", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int32) })
	})

	t.Run("uint32", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return uint32(rand.Int()) })
	})

	t.Run("*uint32", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(uint32) })
	})

	t.Run("int64", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return int64(rand.Int()) })
	})

	t.Run("*int64", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int64) })
	})

	t.Run("uint64", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return uint64(rand.Int()) })
	})

	t.Run("*uint64", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(uint64) })
	})

	t.Run("int", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return int(rand.Int()) })
	})

	t.Run("*int", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(int) })
	})

	t.Run("uint", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return uint(rand.Int()) })
	})

	t.Run("*uint", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return new(uint) })
	})

	t.Run("misaligned struct", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return misaligned{c: rand.Int63()} })
	})

	t.Run("*misaligned struct", func(t *testing.T) {
		runEstimateTest(t, func() interface{} { return &misaligned{} })
	})

	t.Run("*interface", func(t *testing.T) {
		runEstimateTest(t, func() interface{} {
			obj := interface{}(1)
			return &obj
		})
	})
}

func TestEstimateTopLevel(t *testing.T) {
	st := startest.From(t)

	st.RunThread(func(thread *starlark.Thread) {
		str := "just a string"
		obj := []string{}
		for i := 0; i < st.N; i++ {
			obj = append(obj, str)
		}
		thread.AddAllocs(int64(starlark.EstimateSize(obj)))
		st.KeepAlive(obj)
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

			thread.AddAllocs(int64(starlark.EstimateSizeDeep(value)))
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

			thread.AddAllocs(int64(starlark.EstimateSizeDeep(value)))
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
	st := startest.From(t)
	st.RunThread(func(thread *starlark.Thread) {
		value := make(chan int, st.N)

		for i := 0; i < st.N; i++ {
			value <- i
		}

		thread.AddAllocs(int64(starlark.EstimateSizeDeep(value)))
		st.KeepAlive(value)
	})
}

func TestTinyAllocator(t *testing.T) {
	var anchor *int64
	st := startest.From(t)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			anchor = new(int64)
			*anchor = int64(i)

			value := int64(i * 0xcc9e2d51)

			thread.AddAllocs(int64(starlark.EstimateSizeDeep(value)))
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

				thread.AddAllocs(int64(starlark.EstimateSizeDeep(value)))
				st.KeepAlive(value)
			}
		})
	})

	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RunThread(func(thread *starlark.Thread) {
			value := strings.Repeat("Hello World!", st.N)

			thread.AddAllocs(int64(starlark.EstimateSizeDeep(value)))
			st.KeepAlive(value)
		})
	})
}
