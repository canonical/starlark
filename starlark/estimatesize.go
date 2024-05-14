package starlark

import (
	"fmt"
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
//  - estimateXXXAll functions will for a given kind only call:
//    - estimateXXXAll functions OR
//    - estimateXXXDirect and estimateXXXIndirect functions to take their sum.
//
// EstimateSizeShallow has been removed for now since there was no agreement of how
// to make it consistent.

var StringTypeOverhead = EstimateSize("")
var SliceTypeOverhead = EstimateSize([]struct{}{})

// EstimateSize returns the estimated size of the value pointed to by obj, taking
// into account the whole object tree. Where necessary to avoid estimating the
// size of values not controlled by Starlark, objects should implement the
// optional SizeAware interface to override the default exploration.
func EstimateSize(obj interface{}) int64 {
	if obj == nil {
		return 0
	}

	if sizeAware, ok := obj.(SizeAware); ok {
		return sizeAware.EstimateSize()
	}

	v := reflect.ValueOf(obj)

	if v.Kind() == reflect.Ptr {
		return int64(estimateSizeIndirect(v, make(map[uintptr]struct{})))
	}

	return int64(estimateSizeAll(v, make(map[uintptr]struct{})))
}

// EstimateMakeSize estimates the cost of calling make to build a slice, map or
// chan of n elements as specified by template.
func EstimateMakeSize(template interface{}, n int) int64 {
	v := reflect.ValueOf(template)
	switch v.Kind() {
	case reflect.Slice:
		return int64(estimateMakeSliceSize(v, n))
	case reflect.Map:
		return int64(estimateMakeMapSize(v, n))
	case reflect.Chan:
		return int64(estimateMakeChanSize(v, n))
	default:
		panic(fmt.Sprintf("template must be a slice, map or chan: got %s", v.Kind()))
	}
}

const templateTooLong = "template length must be at most 1: got length %d"

func estimateMakeSliceSize(template reflect.Value, n int) uintptr {
	len := template.Len()
	if len > 1 {
		panic(fmt.Sprintf(templateTooLong, len))
	}

	size := roundAllocSize(uintptr(n) * template.Type().Elem().Size())
	if len > 0 {
		size += uintptr(n) * estimateSizeIndirect(template.Index(0), make(map[uintptr]struct{}))
	}
	return size
}

func estimateMakeMapSize(template reflect.Value, n int) uintptr {
	len := template.Len()
	if len > 1 {
		panic(fmt.Sprintf(templateTooLong, len))
	}

	size := estimateMapDirectWithLen(template.Type(), n)
	if len > 0 {
		iter := template.MapRange()
		iter.Next()

		seen := map[uintptr]struct{}{}
		size += uintptr(n) * estimateSizeIndirect(iter.Key(), seen)
		size += uintptr(n) * estimateSizeIndirect(iter.Value(), seen)
	}
	return size
}

func estimateMakeChanSize(template reflect.Value, n int) uintptr {
	if len := template.Len(); len > 1 {
		panic(fmt.Sprintf(templateTooLong, len))
	}
	return estimateChanDirectWithCap(template.Type(), n)
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

	if v.CanInterface() {
		if sizeAware, ok := v.Interface().(SizeAware); ok {
			return uintptr(sizeAware.EstimateSize())
		}
	}

	switch v.Kind() {
	// The following kinds are pointer-like, so their memory lives outside
	// of this already-counted structure. We must therefore estimate the
	// direct and indirect memory of the pointed-to value.
	case reflect.Interface:
		if v.IsNil() {
			return 0
		}

		elem := v.Elem()
		if elem.Kind() == reflect.Ptr {
			return estimateSizeIndirect(elem, seen)
		}

		return estimateSizeAll(elem, seen)
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
	return estimateChanDirectWithCap(v.Type(), v.Cap())
}

func estimateChanDirectWithCap(t reflect.Type, cap int) uintptr {
	// This is a very rough approximation of the size of
	// the chan header.
	const chanHeaderSize = 10 * unsafe.Sizeof(int(0))

	elemSize := t.Elem().Size()

	// The two calls provide a pessimistic view since in case of
	// an elementType that doesn't contain any pointer it
	// will be allocated in a single bigger block (leading
	// to a single getAllocSize call).
	return roundAllocSize(chanHeaderSize) + roundAllocSize(uintptr(cap)*elemSize)
}

func estimateMapAll(v reflect.Value, seen map[uintptr]struct{}) uintptr {
	if v.IsNil() {
		return 0
	}

	ptr := v.Pointer()
	if _, ok := seen[ptr]; ok {
		return 0
	}

	seen[ptr] = struct{}{}
	return estimateMapDirect(v) + estimateMapIndirect(v, seen)
}

func estimateMapDirect(v reflect.Value) uintptr {
	return estimateMapDirectWithLen(v.Type(), v.Len())
}

func estimateMapDirectWithLen(t reflect.Type, len int) uintptr {
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

	keySize := t.Key().Size()
	valueSize := t.Elem().Size()
	k1 := getMapKVPairSize(keySize, valueSize)

	const k2 = 204*unsafe.Sizeof(uintptr(0)) + 280

	return roundAllocSize(uintptr(len)*k1) + k2
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

// roundAllocSize rounds an intended allocation amount to an allocation
// amount which can be made by Go. This function returns at least 16
// bytes due to how small allocations are grouped.
func roundAllocSize(size uintptr) uintptr {
	const tinyAllocMaxSize = 16

	if size == 0 {
		return 0
	} else if size < tinyAllocMaxSize {
		return tinyAllocMaxSize
	} else {
		return roundupsize(size)
	}
}
