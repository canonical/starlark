package starlark

import (
	"reflect"
	"testing"
	"unsafe"
)

type mixed struct { // 12 bytes
	a, b, c int32
}

func TestMeasureValue(t *testing.T) {

	if MeasureValue(bool(false)) != 0 || MeasureValue(int8(0)) != 0 || MeasureValue(uint8(0)) != 0 {
		t.Errorf("One byte structures should not incur in heap allocation")
	}

	// Standard types should be well aligned and not incur in allocation overhead
	if MeasureValue(int16(0)) != unsafe.Sizeof(int16(0)) ||
		MeasureValue(uint16(0)) != unsafe.Sizeof(uint16(0)) ||
		MeasureValue(int32(0)) != unsafe.Sizeof(int32(0)) ||
		MeasureValue(uint32(0)) != unsafe.Sizeof(uint32(0)) ||
		MeasureValue(float32(0)) != unsafe.Sizeof(float32(0)) ||
		MeasureValue(int64(0)) != unsafe.Sizeof(int64(0)) ||
		MeasureValue(uint64(0)) != unsafe.Sizeof(uint64(0)) ||
		MeasureValue(float64(0)) != unsafe.Sizeof(float64(0)) ||
		MeasureValue(complex64(0)) != unsafe.Sizeof(complex64(0)) ||
		MeasureValue(complex128(0)) != unsafe.Sizeof(complex128(0)) ||
		MeasureValue(int(0)) != unsafe.Sizeof(int(0)) ||
		MeasureValue(uint(0)) != unsafe.Sizeof(uint(0)) ||
		MeasureValue(uintptr(0)) != unsafe.Sizeof(uintptr(0)) ||
		MeasureValue(unsafe.Pointer(nil)) != unsafe.Sizeof(unsafe.Pointer(nil)) ||
		MeasureValue(new(int)) != unsafe.Sizeof(new(int)) {

		t.Errorf("Basic data types should not incur in allocation overhead")
	}

	if MeasureValue(mixed{}) <= unsafe.Sizeof(mixed{}) {

		t.Errorf("Measure of oddly sized struct should be greater than struct size (%d/%d)", MeasureValue(mixed{}), unsafe.Sizeof(mixed{}))
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
	if MeasureValue(make([]bool, 16)) != unsafe.Sizeof(bool(false))*16+sliceHeader ||
		MeasureValue(make([]int8, 16)) != unsafe.Sizeof(int8(0))*16+sliceHeader ||
		MeasureValue(make([]uint8, 16)) != unsafe.Sizeof(uint8(0))*16+sliceHeader {

		t.Errorf("Wrong slice size detected")
	}

	// Standard types should be well aligned and not incur in allocation overhead
	if MeasureValue(make([]int16, 16)) != unsafe.Sizeof(int16(0))*16+sliceHeader ||
		MeasureValue(make([]uint16, 16)) != unsafe.Sizeof(uint16(0))*16+sliceHeader ||
		MeasureValue(make([]int32, 16)) != unsafe.Sizeof(int32(0))*16+sliceHeader ||
		MeasureValue(make([]uint32, 16)) != unsafe.Sizeof(uint32(0))*16+sliceHeader ||
		MeasureValue(make([]float32, 16)) != unsafe.Sizeof(float32(0))*16+sliceHeader ||
		MeasureValue(make([]int64, 16)) != unsafe.Sizeof(int64(0))*16+sliceHeader ||
		MeasureValue(make([]uint64, 16)) != unsafe.Sizeof(uint64(0))*16+sliceHeader ||
		MeasureValue(make([]float64, 16)) != unsafe.Sizeof(float64(0))*16+sliceHeader ||
		MeasureValue(make([]complex64, 16)) != unsafe.Sizeof(complex64(0))*16+sliceHeader ||
		MeasureValue(make([]complex128, 16)) != unsafe.Sizeof(complex128(0))*16+sliceHeader ||
		MeasureValue(make([]int, 16)) != unsafe.Sizeof(int(0))*16+sliceHeader ||
		MeasureValue(make([]uint, 16)) != unsafe.Sizeof(uint(0))*16+sliceHeader ||
		MeasureValue(make([]uintptr, 16)) != unsafe.Sizeof(uintptr(0))*16+sliceHeader ||
		MeasureValue(make([]unsafe.Pointer, 16)) != unsafe.Sizeof(unsafe.Pointer(nil))*16+sliceHeader ||
		MeasureValue(make([]*int, 16)) != unsafe.Sizeof(new(int))*16+sliceHeader {

		t.Errorf("Wrong slice size detected")
	}

	if MeasureValue(make([]mixed, 16)) <= unsafe.Sizeof(mixed{})*16+sliceHeader {
		t.Errorf("Wrong slice size detected")
	}
}
