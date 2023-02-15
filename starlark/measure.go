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
//
// As a result we maintain the following convention:
//  - estimateXXXDirect functions only call other estimateXXXDirect functions;
//  - estimateXXXIndirect functions call:
//    - estimateXXXAll functions for pointer-like values and
//    - estimateXXXDirect functions for embedded ones;
//  - estimateXXXAll functions only call estimateXXXAll functions.
//
// EstimateSizeShallow has been removed for now since there was no agreement of how
// to make it consistent.

// EstimateSize returns the estimated size of the
// value pointed by obj, taking into account the whole
// object tree.
func EstimateSize(obj interface{}) uintptr {
	if obj == nil {
		return 0
	}

	return estimateInterfaceAll(reflect.ValueOf(obj), make(map[uintptr]struct{}))
}

func estimateInterfaceAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			if _, ok := seen[v.Pointer()]; !ok {
				return estimateSizeAll(v.Elem(), seen)
			}
		}
		return 0
	default:
		return estimateSizeAll(v, seen)
	}
}

func estimateSizeAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	switch v.Kind() {
	case reflect.String:
		return estimateStringAll(v, seen)
	case reflect.Chan:
		return estimateChanAll(v, seen)
	case reflect.Map:
		return estimateMapAll(v, seen)
	default:
		return estimateSizeDirect(v) + estimateSizeIndirect(v, seen)
	}
}

func estimateSizeDirect(v reflect.Value) uintptr {
	return roundAllocSize(v.Type().Size())
}

func estimateSizeIndirect(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	// This adds the address of the value or the field to the `seen`
	// list. It is important to still consider the memory **pointed
	// by** this memory, so that we don't miss anything (e.g. pointers
	// to the members of an array or members of a struct)
	if v.CanAddr() {
		ptr := v.Addr().Pointer()
		seen[ptr] = struct{}{}
	}

	switch v.Kind() {
	// The following kinds are pointer-like, so their memory lives outside
	// of this already-counted structure. We must therefore estimate the
	// direct and indirect memory of the pointed-to value.
	case reflect.Interface:
		if v.IsNil() {
			return 0
		}

		return estimateInterfaceAll(v.Elem(), seen)
	case reflect.Ptr:
		if !v.IsNil() {
			if _, ok := seen[v.Pointer()]; !ok {
				return estimateSizeAll(v.Elem(), seen)
			}
		}
		return 0
	case reflect.Map:
		return estimateMapAll(v, seen)
	case reflect.Slice:
		return estimateSliceAll(v, seen)
	case reflect.Chan:
		return estimateChanAll(v, seen)

	// The following kinds are embedded, so backing storage
	// has already been accounted for.
	case reflect.Struct:
		return estimateStructIndirect(v, seen)
	case reflect.Array:
		return estimateArrayIndirect(v, seen)
	case reflect.String:
		return estimateStringIndirect(v, seen)
	default:
		return 0
	}
}

func estimateStringAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	// There are misuses of strings.Builder that may lead
	// to some assumptions in this functions being false, for example:
	//
	//   b := &strings.Builder{}
	//   b.Grow(100)
	//   s := b.String()
	//
	// it is not possible to get the capacity of the buffer
	// holding the string.
	if v.Len() == 0 {
		// In this case (excluding the above) neither the memory
		// for the string nor the memory for the header are allocated.
		return 0
	}
	return estimateSizeDirect(v) + estimateStringIndirect(v, seen)
}

func estimateStringIndirect(v reflect.Value, _ map[uintptr]struct{}) uintptr {
	return roundAllocSize(uintptr(v.Len()))
}

func estimateChanAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	if !v.IsNil() {
		ptr := v.Pointer()
		if _, ok := seen[ptr]; !ok {
			seen[ptr] = struct{}{}
			// There is no chan indirect call as the contents of a
			// channel cannot be safely read without flushing them.
			return estimateChanDirect(v)
		}
	}

	return 0
}

func estimateChanDirect(v reflect.Value) uintptr {
	// This is a very rough approximation of the size of
	// the chan header.
	const chanHeaderSize = 10 * unsafe.Sizeof(int(0))

	elementType := v.Type().Elem()

	// The two calls provide a pessimistic view since in case of
	// an elementType that doesn't contain any pointer it
	// will be allocated in a single bigger block (leading
	// to a single getAllocSize call).
	return roundAllocSize(chanHeaderSize) + roundAllocSize(uintptr(v.Cap())*elementType.Size())
}

func estimateMapAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	ptr := v.Pointer()
	if _, ok := seen[ptr]; ok {
		return 0
	}

	seen[ptr] = struct{}{}
	return estimateMapDirect(v) + estimateMapIndirect(v, seen)
}

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
	// y = k1*x + k2
	// Where x is the capacity, y the size and k1, k2 are constants.
	//
	// In detail:
	// - k1 = (size_k + size_v + 1) * 4 + sizeof(ptr)
	// - k2 = 1912 when x64, 1096 when x86

	mapType := v.Type()
	keySize := mapType.Key().Size()
	valueSize := mapType.Elem().Size()
	k1 := getMapKVPairSize(keySize, valueSize)

	const k2 = 204*unsafe.Sizeof(uintptr(0)) + 280

	return roundAllocSize(uintptr(v.Len())*k1) + k2
}

// getMapKVPairSize returns the estimated size a key-value pair
// would take when used inside a go map.
func getMapKVPairSize(k, v uintptr) uintptr {
	// There is a little complexity here: if the key and the
	// value are too big they get allocated separately and only
	// the pointer is stored.
	const maxElementSize = 128
	if k < maxElementSize && v < maxElementSize {
		return (k+v+1)*4 + unsafe.Sizeof(uintptr(0))
	} else if k < maxElementSize {
		return getMapKVPairSize(k, 8) + v
	} else if v < maxElementSize {
		return getMapKVPairSize(8, v) + k
	} else {
		return getMapKVPairSize(8, 8) + k + v
	}
}

func estimateMapIndirect(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	var result uintptr

	iter := v.MapRange()
	for iter.Next() {
		result += estimateSizeIndirect(iter.Key(), seen)
		result += estimateSizeIndirect(iter.Value(), seen)
	}

	return result
}

func estimateSliceAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	if v.IsNil() {
		return 0
	}

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
}

func estimateSliceDirect(v reflect.Value) uintptr {
	return roundAllocSize(v.Type().Elem().Size() * uintptr(v.Cap()))
}

func estimateSliceIndirect(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	result := uintptr(0)

	for i := 0; i < v.Len(); i++ {
		result += estimateSizeIndirect(v.Index(i), seen)
	}

	return result
}

func estimateArrayIndirect(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	result := uintptr(0)

	for i := 0; i < v.Len(); i++ {
		result += estimateSizeIndirect(v.Index(i), seen)
	}

	return result
}

func estimateStructIndirect(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	result := uintptr(0)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		result += estimateSizeIndirect(field, seen)
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

// roundAllocSize rounds an intended allocation amount to an allocation
// amount which can be made by Go. This function returns at least 16
// bytes due to how small allocations are grouped.
func roundAllocSize(size uintptr) uintptr {
	// This is the same as `runtime.roundupsize`
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
