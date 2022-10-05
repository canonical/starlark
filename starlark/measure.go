package starlark

import (
	"reflect"
	"unsafe"
)

const (
	maxSmallSize   = 32768
	smallSizeDiv   = 8
	smallSizeMax   = 1024
	largeSizeDiv   = 128
	numSizeClasses = 68
	interfaceSize  = unsafe.Sizeof(interface{}(nil))
)

var class_to_size = [numSizeClasses]uint16{0, 8, 16, 24, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192, 208, 224, 240, 256, 288, 320, 352, 384, 416, 448, 480, 512, 576, 640, 704, 768, 896, 1024, 1152, 1280, 1408, 1536, 1792, 2048, 2304, 2688, 3072, 3200, 3456, 4096, 4864, 5376, 6144, 6528, 6784, 6912, 8192, 9472, 9728, 10240, 10880, 12288, 13568, 14336, 16384, 18432, 19072, 20480, 21760, 24576, 27264, 28672, 32768}
var size_to_class8 = [smallSizeMax/smallSizeDiv + 1]uint8{0, 1, 2, 3, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 16, 16, 17, 17, 18, 18, 19, 19, 19, 19, 20, 20, 20, 20, 21, 21, 21, 21, 22, 22, 22, 22, 23, 23, 23, 23, 24, 24, 24, 24, 25, 25, 25, 25, 26, 26, 26, 26, 27, 27, 27, 27, 27, 27, 27, 27, 28, 28, 28, 28, 28, 28, 28, 28, 29, 29, 29, 29, 29, 29, 29, 29, 30, 30, 30, 30, 30, 30, 30, 30, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32}
var size_to_class128 = [(maxSmallSize-smallSizeMax)/largeSizeDiv + 1]uint8{32, 33, 34, 35, 36, 37, 37, 38, 38, 39, 39, 40, 40, 40, 41, 41, 41, 42, 43, 43, 44, 44, 44, 44, 44, 45, 45, 45, 45, 45, 45, 46, 46, 46, 46, 47, 47, 47, 47, 47, 47, 48, 48, 48, 49, 49, 50, 51, 51, 51, 51, 51, 51, 51, 51, 51, 51, 52, 52, 52, 52, 52, 52, 52, 52, 52, 52, 53, 53, 54, 54, 54, 54, 55, 55, 55, 55, 55, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 57, 57, 57, 57, 57, 57, 57, 57, 57, 57, 58, 58, 58, 58, 58, 58, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 61, 61, 61, 61, 61, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 63, 63, 63, 63, 63, 63, 63, 63, 63, 63, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67}

func divRoundUp(n, a uintptr) uintptr {
	return (n + a - 1) / a
}

func GetAllocSize(size uintptr) uintptr {
	var sizeclass uint8
	if size <= smallSizeMax-8 {
		sizeclass = size_to_class8[divRoundUp(size, smallSizeDiv)]
	} else {
		sizeclass = size_to_class128[divRoundUp(size-smallSizeMax, largeSizeDiv)]
	}
	return uintptr(class_to_size[sizeclass])
}

func measureType(t reflect.Type) uintptr {
	switch t.Kind() {
	case reflect.Bool, reflect.Int8, reflect.Uint8:
		return 0

	case reflect.Int, reflect.Uint, reflect.Uintptr,
		reflect.UnsafePointer, reflect.Ptr, reflect.Int16,
		reflect.Uint16, reflect.Int32, reflect.Uint32,
		reflect.Float32, reflect.Int64, reflect.Uint64,
		reflect.Float64, reflect.Complex64, reflect.Complex128:
		return t.Size()

	case reflect.Struct, reflect.Array:
		return GetAllocSize(t.Size())

	case reflect.Interface:
		return interfaceSize

	case reflect.Slice:
		return unsafe.Sizeof(reflect.SliceHeader{})

	case reflect.String:
		return unsafe.Sizeof(reflect.StringHeader{})
	}

	panic("Unknown type")
}

func MeasureType(obj interface{}) uintptr {
	return measureType(reflect.TypeOf(obj))
}

func max(a, b uintptr) uintptr {
	if a > b {
		return a
	} else {
		return b
	}
}

func measureValue(v reflect.Value, recursive bool) uintptr {
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map:
		// HOW?
		panic("WTF")

	case reflect.Slice:
		elementType := v.Type().Elem()
		elementSize := max(measureType(elementType), elementType.Size())
		return unsafe.Sizeof(reflect.SliceHeader{}) + GetAllocSize(elementSize*uintptr(v.Cap()))

	case reflect.String:
		return unsafe.Sizeof(reflect.StringHeader{}) + GetAllocSize(uintptr(v.Len()))
	default:
		return measureType(v.Type())
	}
}

func MeasureValue(obj interface{}) uintptr {
	return measureValue(reflect.ValueOf(obj), false)
}

func MeasureValueDeep(obj interface{}) uintptr {
	return measureValue(reflect.ValueOf(obj), true)
}
