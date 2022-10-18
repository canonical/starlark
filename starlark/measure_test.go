package starlark_test

import (
	"math/rand"
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
			st.Track(createObj())
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
			st.Track(value)
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
			st.Track(value)
		})
	})
}
