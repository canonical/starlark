package starlark

import (
	"fmt"
	"reflect"
)

type SafeAppender struct {
	thread   *Thread
	slice    reflect.Value
	elemType reflect.Type
	allocs   SafeInteger
	steps    int64
}

func NewSafeAppender(thread *Thread, slicePtr interface{}) *SafeAppender {
	if slicePtr == nil {
		panic("NewSafeAppender: expected pointer to slice, got nil")
	}
	ptr := reflect.ValueOf(slicePtr)
	if kind := ptr.Kind(); kind != reflect.Ptr {
		panic(fmt.Sprintf("NewSafeAppender: expected pointer to slice, got %v", kind))
	}
	slice := ptr.Elem()
	if kind := slice.Kind(); kind != reflect.Slice {
		panic(fmt.Sprintf("NewSafeAppender: expected pointer to slice, got pointer to %v", kind))
	}

	elemType := slice.Type().Elem()
	return &SafeAppender{
		thread:   thread,
		slice:    slice,
		elemType: elemType,
	}
}

// Allocs returns the total allocations reported to this SafeAppender's thread.
func (sa *SafeAppender) Allocs() SafeInteger {
	return sa.allocs
}

// Steps returns the total steps reported to this SafeAppender's thread.
func (sa *SafeAppender) Steps() int64 {
	return sa.steps
}

func (sa *SafeAppender) Append(values ...interface{}) error {
	if sa.thread != nil {
		if err := sa.thread.AddSteps(SafeInt(len(values))); err != nil {
			return err
		}
	}
	sa.steps = OldSafeAdd64(sa.steps, int64(len(values)))

	cap := sa.slice.Cap()
	newSize := sa.slice.Len() + len(values)
	if newSize > cap && sa.thread != nil {
		if err := sa.thread.CheckAllocs(OldSafeMul64(int64(newSize), int64(sa.elemType.Size()))); err != nil {
			return err
		}
	}
	slice := sa.slice
	for _, value := range values {
		if value == nil {
			switch sa.elemType.Kind() {
			case reflect.Chan, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
			default:
				panic("SafeAppender.Append: unexpected nil")
			}
			slice = reflect.Append(slice, reflect.Zero(sa.elemType))
		} else {
			slice = reflect.Append(slice, reflect.ValueOf(value))
		}
	}
	if slice.Cap() != cap && sa.thread != nil {
		// TODO(kcza): Compute roundAllocSize arguments safely.
		oldSize := roundAllocSize(int64(cap) * int64(sa.elemType.Size()))
		newSize := roundAllocSize(int64(slice.Cap()) * int64(sa.elemType.Size()))
		delta := SafeSub(newSize, oldSize)
		sa.allocs = SafeAdd(sa.allocs, delta)
		if err := sa.thread.AddAllocs(delta); err != nil {
			return err
		}
	}
	sa.slice.Set(slice)
	return nil
}

func (sa *SafeAppender) AppendSlice(values interface{}) error {
	if values == nil {
		panic("SafeAppender.AppendSlice: expected slice, got nil")
	}
	toAppend := reflect.ValueOf(values)
	if kind := toAppend.Kind(); kind != reflect.Slice {
		panic(fmt.Sprintf("SafeAppender.AppendSlice: expected slice, got %v", kind))
	}
	if sa.thread != nil {
		if err := sa.thread.AddSteps(SafeInt(toAppend.Len())); err != nil {
			return err
		}
	}
	sa.steps = OldSafeAdd64(sa.steps, int64(toAppend.Len()))

	len := sa.slice.Len()
	cap := sa.slice.Cap()
	toAppendLen := toAppend.Len()
	if newLen := OldSafeAdd(len, toAppendLen); newLen > cap && sa.thread != nil {
		// Consider up to twice the size for the allocation overshoot
		allocation := OldSafeAdd64(OldSafeMul64(int64(newLen), 2), -int64(cap))
		if err := sa.thread.CheckAllocs(int64(OldSafeMul64(allocation, int64(sa.elemType.Size())))); err != nil {
			return err
		}
	}

	slice := reflect.AppendSlice(sa.slice, toAppend)
	if slice.Cap() != cap && sa.thread != nil {
		// TODO(kcza): Compute roundAllocSize arguments safely.
		oldSize := roundAllocSize(OldSafeMul64(int64(cap), int64(sa.elemType.Size())))
		newSize := roundAllocSize(OldSafeMul64(int64(slice.Cap()), int64(sa.elemType.Size())))
		delta := SafeSub(newSize, oldSize)
		sa.allocs = SafeAdd(sa.allocs, delta)
		if err := sa.thread.AddAllocs(delta); err != nil {
			return err
		}
	}
	sa.slice.Set(slice)
	return nil
}
