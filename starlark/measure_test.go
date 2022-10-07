package starlark_test

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/canonical/starlark/starlark"
)

type mixed struct { // 12 bytes
	a, b, c int32
}

func TestMeasureValue(t *testing.T) {

	if starlark.MeasureValue(bool(false)) != 0 || starlark.MeasureValue(int8(0)) != 0 || starlark.MeasureValue(uint8(0)) != 0 {
		t.Errorf("One byte structures should not incur in heap allocation")
	}

	// Standard types should be well aligned and not incur in allocation overhead
	if starlark.MeasureValue(int16(0)) != unsafe.Sizeof(int16(0)) ||
		starlark.MeasureValue(uint16(0)) != unsafe.Sizeof(uint16(0)) ||
		starlark.MeasureValue(int32(0)) != unsafe.Sizeof(int32(0)) ||
		starlark.MeasureValue(uint32(0)) != unsafe.Sizeof(uint32(0)) ||
		starlark.MeasureValue(float32(0)) != unsafe.Sizeof(float32(0)) ||
		starlark.MeasureValue(int64(0)) != unsafe.Sizeof(int64(0)) ||
		starlark.MeasureValue(uint64(0)) != unsafe.Sizeof(uint64(0)) ||
		starlark.MeasureValue(float64(0)) != unsafe.Sizeof(float64(0)) ||
		starlark.MeasureValue(complex64(0)) != unsafe.Sizeof(complex64(0)) ||
		starlark.MeasureValue(complex128(0)) != unsafe.Sizeof(complex128(0)) ||
		starlark.MeasureValue(int(0)) != unsafe.Sizeof(int(0)) ||
		starlark.MeasureValue(uint(0)) != unsafe.Sizeof(uint(0)) ||
		starlark.MeasureValue(uintptr(0)) != unsafe.Sizeof(uintptr(0)) ||
		starlark.MeasureValue(unsafe.Pointer(nil)) != unsafe.Sizeof(unsafe.Pointer(nil)) ||
		starlark.MeasureValue(new(int)) != unsafe.Sizeof(new(int)) {

		t.Errorf("Basic data types should not incur in allocation overhead")
	}

	if starlark.MeasureValue(mixed{}) <= unsafe.Sizeof(mixed{}) {

		t.Errorf("Measure of oddly sized struct should be greater than struct size (%d/%d)", starlark.MeasureValue(mixed{}), unsafe.Sizeof(mixed{}))
	}
}

func TestMeasureSlices(t *testing.T) {
	// When represented as an interface a slice is laid out in memory like:
	//
	// Interface         SliceHeader
	// +-----------+     +----------+        +---------+
	// | value Ptr |---->| data Ptr |------->|  Data   |
	// | type Type |     | len int  |        +---------+
	// +-----------+     | cap int  |
	// 				     +----------+
	// Of course, more than one SliceHeader could point to the same data.
	// In this case, MeasureValue assumes complete ownership of the slice.
	const sliceHeader = unsafe.Sizeof(reflect.SliceHeader{})

	// The slice size should always be a multiplier of the struct size
	if starlark.MeasureValue(make([]bool, 16)) != unsafe.Sizeof(bool(false))*16+sliceHeader ||
		starlark.MeasureValue(make([]int8, 16)) != unsafe.Sizeof(int8(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]uint8, 16)) != unsafe.Sizeof(uint8(0))*16+sliceHeader {

		t.Errorf("Wrong slice size detected")
	}

	// Standard types should be well aligned and not incur in allocation overhead
	if starlark.MeasureValue(make([]int16, 16)) != unsafe.Sizeof(int16(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]uint16, 16)) != unsafe.Sizeof(uint16(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]int32, 16)) != unsafe.Sizeof(int32(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]uint32, 16)) != unsafe.Sizeof(uint32(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]float32, 16)) != unsafe.Sizeof(float32(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]int64, 16)) != unsafe.Sizeof(int64(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]uint64, 16)) != unsafe.Sizeof(uint64(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]float64, 16)) != unsafe.Sizeof(float64(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]complex64, 16)) != unsafe.Sizeof(complex64(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]complex128, 16)) != unsafe.Sizeof(complex128(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]int, 16)) != unsafe.Sizeof(int(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]uint, 16)) != unsafe.Sizeof(uint(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]uintptr, 16)) != unsafe.Sizeof(uintptr(0))*16+sliceHeader ||
		starlark.MeasureValue(make([]unsafe.Pointer, 16)) != unsafe.Sizeof(unsafe.Pointer(nil))*16+sliceHeader ||
		starlark.MeasureValue(make([]*int, 16)) != unsafe.Sizeof(new(int))*16+sliceHeader {

		t.Errorf("Wrong slice size detected")
	}

	if starlark.MeasureValue(make([]mixed, 16)) <= unsafe.Sizeof(mixed{})*16+sliceHeader {
		t.Errorf("Wrong slice size detected")
	}
}
