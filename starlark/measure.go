package starlark

import (
	"reflect"
	"unsafe"
)

// Functions in this file each handle a single case for estimating the size of an
// object. As a general rule, each function assumes that the memory of the topmost
// structure has already been counted. These functions fall into three categories.
//
// Those named estimateXXXDirect estimate the memory relating to how type XXX is
// represented. This is more "extended" than simply unsafe.SizeOf, for example, a
// map would include the backing memory for the entries and a slice would include
// the backing array.
//
// Those named estimateXXXIndirect estimate the memory of what's contained by an
// XXX, rather than that used to represent XXX itself. For example, these
// functions measure elements (and their children) in a slice rather than the
// slice's backing array.
//
// Where appropriate, functions named estimateXXXAll are provided to improve
// readability. They return the sum of estimateXXXDirect and estimateXXXIndirect.

// EstimateSize returns the estimated size of the value
// pointed by obj, without taking into account eventual
// nested members.
func EstimateSize(obj interface{}) uintptr {
	if obj == nil {
		return 0
	} else {
		return estimateInterface(reflect.ValueOf(obj), nil)
	}
}

// EstimateSizeDeep returns the estimated size of the
// value pointed by obj, taking into account the whole
// object tree.
func EstimateSizeDeep(obj interface{}) uintptr {
	if obj == nil {
		return 0
	} else {
		return estimateInterface(reflect.ValueOf(obj), make(map[uintptr]struct{}))
	}
}

// estimateInterface returns the estimated size of the content of an interface
func estimateInterface(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	if v.Kind() == reflect.String {
		// In this case neither the memory for the string nor
		// the memory for the header are allocated.
		// TODO: consider other cases where a value is not allocated
		// like empty slices and zeroed structs.
		if v.Len() == 0 {
			return 0
		}
	}

	result := estimateSize(v, seen)

	// In the case of a pointer, the entire value is stored as the pointer of
	// the interface, so it shouldn't be counted. In all other cases it will
	// take some form of address. Maps and Funcs are to be considered pointers.
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Func:
		return result
	default:
		return getAllocSize(v.Type().Size()) + result
	}
}

// estimateSize estimates the size of the given value. If map of seen
// addresses is nil, it will exclude indirectly-related memory.
func estimateSize(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	// FIXME strings are counted multiple times
	if seen != nil {
		// This adds the address of the value or the field to the `seen`
		// list. It is important to still consider the memory **pointed
		// by** this memory, so that we don't miss anything (e.g. pointers
		// to the members of an array or members of a struct)
		if v.CanAddr() {
			ptr := v.Addr().Pointer()
			seen[ptr] = struct{}{}
		}

		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				return estimateInterface(v.Elem(), seen)
			} else {
				return 0
			}

		case reflect.Ptr:
			if !v.IsNil() {
				if _, ok := seen[v.Pointer()]; !ok {
					elem := v.Elem()
					return estimateSize(elem, seen) + getAllocSize(elem.Type().Size())
				}
			}

			return 0

		case reflect.Map:
			return estimateMapAll(v, seen)

		case reflect.Slice:
			return estimateSliceAll(v, seen)

		case reflect.Chan:
			return estimateChanAll(v, seen)

		case reflect.Struct:
			return estimateStructIndirect(v, seen)

		case reflect.Array:
			return estimateSliceIndirect(v, seen)

		case reflect.String:
			return getAllocSize(uintptr(v.Len()))
		}
	} else {
		// In the case of slices, maps and strings we count the first level of memory
		switch v.Kind() {
		case reflect.Map:
			return estimateMapDirect(v)

		case reflect.Slice:
			return estimateSliceDirect(v)

		case reflect.Chan:
			return estimateChanDirect(v)

		case reflect.String:
			return getAllocSize(uintptr(v.Len()))

		case reflect.Ptr:
			return getAllocSize(v.Elem().Type().Size())
		}
	}

	return 0
}

// estimateChanAll returns the estimated size for the channel buffer, if
// not already visited.
func estimateChanAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	if !v.IsNil() {
		ptr := v.Pointer()
		if _, ok := seen[ptr]; !ok {
			seen[ptr] = struct{}{}
			return estimateChanDirect(v)
		}
	}

	return 0
}

// estimateChanDirect returns the estimated size for the channel buffer.
// It doesn't include any storage for the indirects pointed by
// the values.
func estimateChanDirect(v reflect.Value) uintptr {
	const chanStructSize = 10 * unsafe.Sizeof(int(0))

	elementType := v.Type().Elem()
	// This is a pessimistic view since in case of
	// an elementType that doesn't contain any pointer it
	// will be allocated in a single bigger block (leading
	// to a single getAllocSize call).
	return getAllocSize(chanStructSize) + getAllocSize(uintptr(v.Cap())*elementType.Size())
}

// estimateMapAll returns the estimated size of both the memory
// used inside of a map and all the indirects stored in its
// keys and values.
func estimateMapAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	ptr := v.Pointer()
	if _, ok := seen[ptr]; ok {
		return 0
	} else {
		seen[ptr] = struct{}{}
		return estimateMapDirect(v) + estimateMapIndirect(v, seen)
	}
}

// estimateMapDirect returns the estimated size of the memory
// used inside a map. This size includes the memory for
// the keys, but not for the indirects pointed by them.
func estimateMapDirect(v reflect.Value) uintptr {
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

	k2 := getMapK2(keySize, valueSize)

	result := getAllocSize(uintptr(v.Len())*k2) + k1

	return result
}

// getMapK2 returns the estimated size a key-value pair
// would take when used inside a go map.
func getMapK2(k, v uintptr) uintptr {
	// There is a little complexity here : if the key and the
	// value are too big they get allocated separately and only
	// the pointer is stored.
	const maxElementSize = 128
	if k < maxElementSize && v < maxElementSize {
		return (k+v+1)*4 + unsafe.Sizeof(uintptr(0))
	} else if k < maxElementSize {
		return getMapK2(k, 8) + v
	} else if v < maxElementSize {
		return getMapK2(8, v) + k
	} else {
		return getMapK2(8, 8) + k + v
	}
}

// estimateMapIndirect returns the estimated size of the indirects
// contained by a map. The size of the Key or Values **is not** counted
// here.
func estimateMapIndirect(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	var result uintptr

	iter := v.MapRange()
	for iter.Next() {
		result += estimateSize(iter.Key(), seen)
		result += estimateSize(iter.Value(), seen)
	}

	return result
}

// estimateSliceAll returns the estimated size for the slice's backing array
// and for all the indirects it contains.
func estimateSliceAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	if !v.IsNil() {
		// FIXME slices are counted multiple times.
		// This function doesn't check if the backing array has already been
		// counted. For example:
		//  a := [16]int{}
		//  b := [][]int { a[0:1:1], a[:] }
		//
		// Both b[0] and b[1] point to the same backing array, but marking that
		// as "seen" while visiting b[0] will make the function miss all the
		// memory pointed by b. It is better in this case to just be pessimistic
		// and estimate more memory than it actually is allocated.
		return estimateSliceDirect(v) + estimateSliceIndirect(v, seen)
	} else {
		return 0
	}
}

// estimateSliceDirect returns the estimated size for the slice backing buffer.
// It doesn't include any storage for the indirects pointed by the values.
func estimateSliceDirect(v reflect.Value) uintptr {
	return getAllocSize(v.Type().Elem().Size() * uintptr(v.Cap()))
}

// estimateSliceIndirect returns the estimated size of the indirects
// contained by an array or a slice. As such, this function will
// panic in case v is not an array or slice.
func estimateSliceIndirect(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	result := uintptr(0)

	for i := 0; i < v.Len(); i++ {
		result += estimateSize(v.Index(i), seen)
	}

	return result
}

// estimateStructIndirect returns the estimated size of indirects
// contained by a struct field. v is expected to be a struct and
// this function will panic in case it's not.
func estimateStructIndirect(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	result := uintptr(0)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		result += estimateSize(field, seen)
	}

	return result
}

const (
	tinyAllocMaxSize = 16
	maxSmallSize     = 32768
	smallSizeDiv     = 8
	smallSizeMax     = 1024
	largeSizeDiv     = 128
	numSizeClasses   = 68
	pageShift        = 13
	pageSize         = 1 << pageShift
)

// These numbers come from the current Go GC implementation https://github.com/golang/go/blob/go1.16/src/runtime/sizeclasses.go#L84
var classToSize = [numSizeClasses]uint16{0, 8, 16, 24, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192, 208, 224, 240, 256, 288, 320, 352, 384, 416, 448, 480, 512, 576, 640, 704, 768, 896, 1024, 1152, 1280, 1408, 1536, 1792, 2048, 2304, 2688, 3072, 3200, 3456, 4096, 4864, 5376, 6144, 6528, 6784, 6912, 8192, 9472, 9728, 10240, 10880, 12288, 13568, 14336, 16384, 18432, 19072, 20480, 21760, 24576, 27264, 28672, 32768}
var sizeToClass8 = [smallSizeMax/smallSizeDiv + 1]uint8{0, 1, 2, 3, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 16, 16, 17, 17, 18, 18, 19, 19, 19, 19, 20, 20, 20, 20, 21, 21, 21, 21, 22, 22, 22, 22, 23, 23, 23, 23, 24, 24, 24, 24, 25, 25, 25, 25, 26, 26, 26, 26, 27, 27, 27, 27, 27, 27, 27, 27, 28, 28, 28, 28, 28, 28, 28, 28, 29, 29, 29, 29, 29, 29, 29, 29, 30, 30, 30, 30, 30, 30, 30, 30, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32}
var sizeToClass128 = [(maxSmallSize-smallSizeMax)/largeSizeDiv + 1]uint8{32, 33, 34, 35, 36, 37, 37, 38, 38, 39, 39, 40, 40, 40, 41, 41, 41, 42, 43, 43, 44, 44, 44, 44, 44, 45, 45, 45, 45, 45, 45, 46, 46, 46, 46, 47, 47, 47, 47, 47, 47, 48, 48, 48, 49, 49, 50, 51, 51, 51, 51, 51, 51, 51, 51, 51, 51, 52, 52, 52, 52, 52, 52, 52, 52, 52, 52, 53, 53, 54, 54, 54, 54, 55, 55, 55, 55, 55, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 57, 57, 57, 57, 57, 57, 57, 57, 57, 57, 58, 58, 58, 58, 58, 58, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 61, 61, 61, 61, 61, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 63, 63, 63, 63, 63, 63, 63, 63, 63, 63, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67}

func alignUp(n, a uintptr) uintptr {
	return (n + a - 1) &^ (a - 1)
}

func divRoundUp(n, a uintptr) uintptr {
	return (n + a - 1) / a
}

// getAllocSize rounds an intended allocation amount to an allocation
// amount which can be made by Go.
func getAllocSize(size uintptr) uintptr {
	if size == 0 {
		return 0
	} else if size < tinyAllocMaxSize {
		// Pessimistic view to take into account linked-lifetimes of
		// transient values in tiny allocator.
		return tinyAllocMaxSize
	} else if size < maxSmallSize {
		if size <= smallSizeMax-8 {
			return uintptr(classToSize[sizeToClass8[divRoundUp(size, smallSizeDiv)]])
		} else {
			return uintptr(classToSize[sizeToClass128[divRoundUp(size-smallSizeMax, largeSizeDiv)]])
		}
	}
	if size+pageSize < size {
		return size
	}
	return alignUp(size, pageSize)
}
