package starlark

import (
	"math/bits"
	"reflect"
	"unsafe"
)

const (
	maxSmallSize   = 32768
	smallSizeDiv   = 8
	smallSizeMax   = 1024
	largeSizeDiv   = 128
	numSizeClasses = 68
	pageShift      = 13
	pageSize       = 1 << pageShift
	interfaceSize  = unsafe.Sizeof(interface{}(nil))
)

var class_to_size = [numSizeClasses]uint16{0, 8, 16, 24, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192, 208, 224, 240, 256, 288, 320, 352, 384, 416, 448, 480, 512, 576, 640, 704, 768, 896, 1024, 1152, 1280, 1408, 1536, 1792, 2048, 2304, 2688, 3072, 3200, 3456, 4096, 4864, 5376, 6144, 6528, 6784, 6912, 8192, 9472, 9728, 10240, 10880, 12288, 13568, 14336, 16384, 18432, 19072, 20480, 21760, 24576, 27264, 28672, 32768}
var size_to_class8 = [smallSizeMax/smallSizeDiv + 1]uint8{0, 1, 2, 3, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 16, 16, 17, 17, 18, 18, 19, 19, 19, 19, 20, 20, 20, 20, 21, 21, 21, 21, 22, 22, 22, 22, 23, 23, 23, 23, 24, 24, 24, 24, 25, 25, 25, 25, 26, 26, 26, 26, 27, 27, 27, 27, 27, 27, 27, 27, 28, 28, 28, 28, 28, 28, 28, 28, 29, 29, 29, 29, 29, 29, 29, 29, 30, 30, 30, 30, 30, 30, 30, 30, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32}
var size_to_class128 = [(maxSmallSize-smallSizeMax)/largeSizeDiv + 1]uint8{32, 33, 34, 35, 36, 37, 37, 38, 38, 39, 39, 40, 40, 40, 41, 41, 41, 42, 43, 43, 44, 44, 44, 44, 44, 45, 45, 45, 45, 45, 45, 46, 46, 46, 46, 47, 47, 47, 47, 47, 47, 48, 48, 48, 49, 49, 50, 51, 51, 51, 51, 51, 51, 51, 51, 51, 51, 52, 52, 52, 52, 52, 52, 52, 52, 52, 52, 53, 53, 54, 54, 54, 54, 55, 55, 55, 55, 55, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 57, 57, 57, 57, 57, 57, 57, 57, 57, 57, 58, 58, 58, 58, 58, 58, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 61, 61, 61, 61, 61, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 63, 63, 63, 63, 63, 63, 63, 63, 63, 63, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67}

const (
	chanSize = 10 * unsafe.Sizeof(int(0))
)

func alignUp(n, a uintptr) uintptr {
	return (n + a - 1) &^ (a - 1)
}

func divRoundUp(n, a uintptr) uintptr {
	return (n + a - 1) / a
}

func GetAllocSize(size uintptr) uintptr {
	if size < maxSmallSize {
		if size <= smallSizeMax-8 {
			return uintptr(class_to_size[size_to_class8[divRoundUp(size, smallSizeDiv)]])
		} else {
			return uintptr(class_to_size[size_to_class128[divRoundUp(size-smallSizeMax, largeSizeDiv)]])
		}
	}
	if size+pageSize < size {
		return size
	}
	return alignUp(size, pageSize)
}

func measureType(t reflect.Type) uintptr {
	switch t.Kind() {
	case reflect.Bool, reflect.Int8, reflect.Uint8:
		return 0

	case reflect.Int, reflect.Uint, reflect.Uintptr,
		reflect.UnsafePointer, reflect.Ptr, reflect.Int16,
		reflect.Uint16, reflect.Int32, reflect.Uint32,
		reflect.Float32, reflect.Int64, reflect.Uint64,
		reflect.Float64, reflect.Complex64, reflect.Complex128,
		reflect.Struct, reflect.Array, reflect.Map,
		reflect.Chan, reflect.Func:
		return GetAllocSize(t.Size())

	case reflect.Interface:
		return interfaceSize
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

func nextPow2(a uintptr) uintptr {
	return 1 << (bits.UintSize - bits.LeadingZeros(uint(a)))
}

// There is a little complexity here : if the key and the
// value are too big they get allocated separately and only
// the pointer is stored.
func getK2(k, v uintptr) uintptr {
	const maxElementSize = 128
	if k < 128 && v < 128 {
		return (k+v+1)*4 + unsafe.Sizeof(uintptr(0))
	} else if k < 128 {
		return getK2(k, 8) + v
	} else if v < 128 {
		return getK2(8, v) + k
	} else {
		return getK2(8, 8) + k + v
	}
}

// Maps are hard to measure because we don't have access
// to the internal capacity (whatever that means). That is
// the first problem: "capacity" is a fuzzy concept in hash
// maps and thus Go team decided not to give a "real" meaning
// to it (word of Ian Lance Taylor). In fact when doing a
// `make(map[<tkey>]<tvalue>, <cap>)`, <cap> is to be considered
// a "hint" to the map. Moreover, AFAIK go maps are grow only.
// This makes it impossible to estimate (even pessimistically)
// the real amount of memory used. We will need to have a
// "usual size" kind of estimation.
// For this reason, we decided to take the "experimental" route,
// collecting data from different classes of key/value sizes
// and finding a pessimistic allocating function.
func measureMap(v reflect.Value, loops map[uintptr]struct{}) uintptr {
	// The approximation is just a line in the form of
	// y = k1 + k2*x
	// Where x is the capacity, y the size and k1, k2 are constants.
	//
	// In detail:
	// - k1 = 1912 when x64, 1096 when x86
	// - k2 = (size_k + size_v + 1) * 4 + sizeof(ptr)
	const (
		k1             = 204*unsafe.Sizeof(uintptr(0)) + 280
		maxElementSize = 128
	)

	mapType := v.Type()
	keySize := mapType.Key().Size()
	valueSize := mapType.Elem().Size()

	k2 := uintptr(0) // GetAllocSize(keySize) + GetAllocSize(valueSize)

	if keySize > maxElementSize {
		k2 += unsafe.Sizeof(uintptr(0))
	}

	if valueSize > maxElementSize {
		k2 += unsafe.Sizeof(uintptr(0))
	}

	result := GetAllocSize(uintptr(v.Len())*k2) + k1

	if loops != nil {
		// Now visit all key-value pairs.
		iter := v.MapRange()
		for iter.Next() {
			result += measureValue(iter.Key(), loops)
			result += measureValue(iter.Value(), loops)
		}
	}

	return result
}


func measureValue(v reflect.Value, loops map[uintptr]struct{}) uintptr {
	if loops != nil {
		switch v.Kind() {
		case reflect.Ptr, reflect.Chan, reflect.Map, reflect.UnsafePointer, reflect.Slice:
			ptr := v.Pointer()
			if _, ok := loops[ptr]; ok {
				return 0 // Already passed here
			} else {
				loops[ptr] = struct{}{}
				defer delete(loops, ptr)
			}
		}
	}

	switch v.Kind() {
	case reflect.Chan:
		elementType := v.Type().Elem()
		// This is a pessimistic view since in case of
		// an elementType that doesn't contain any pointer it
		// will be allocated in a single bigger block (leading
		// to a single GetAllocSize call).
		return GetAllocSize(chanSize) + GetAllocSize(uintptr(v.Cap())*elementType.Size())
	case reflect.Map:
		return measureMap(v, loops)

	case reflect.Slice:
		elementType := v.Type().Elem()
		elementSize := max(measureType(elementType), elementType.Size())
		return v.Type().Size() + GetAllocSize(elementSize*uintptr(v.Cap()))

	case reflect.String:
		return v.Type().Size() + GetAllocSize(uintptr(v.Len()))

	default:
		return measureType(v.Type())
	}
}

func MeasureValue(obj interface{}) uintptr {
	return measureValue(reflect.ValueOf(obj), nil)
}

func MeasureValueDeep(obj interface{}) uintptr {
	return measureValue(reflect.ValueOf(obj), make(map[uintptr]struct{}))
}
