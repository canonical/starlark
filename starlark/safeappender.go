package starlark

import (
	"fmt"
	"reflect"
)

type SafeAppender struct {
	thread        *Thread
	slice         reflect.Value
	elemType      reflect.Type
	allocs, steps int64
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
func (sa *SafeAppender) Allocs() int64 {
	return sa.allocs
}

// Steps returns the total steps reported to this SafeAppender's thread.
func (sa *SafeAppender) Steps() int64 {
	return sa.steps
}

func (sa *SafeAppender) Append(values ...interface{}) error {
	if sa.thread != nil {
		if err := sa.thread.AddSteps(int64(len(values))); err != nil {
			return err
		}
	}
	sa.steps += int64(len(values))

	cap := sa.slice.Cap()
	newSize := sa.slice.Len() + len(values)
	if newSize > cap && sa.thread != nil {
		if err := sa.thread.CheckAllocs(int64(uintptr(newSize) * sa.elemType.Size())); err != nil {
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
		oldSize := roundAllocSize(int64(cap) * int64(sa.elemType.Size()))
		newSize := roundAllocSize(int64(slice.Cap()) * int64(sa.elemType.Size()))
		delta := newSize - oldSize
		sa.allocs += int64(delta)
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
		if err := sa.thread.AddSteps(int64(toAppend.Len())); err != nil {
			return err
		}
	}
	sa.steps += int64(toAppend.Len())

	cap := sa.slice.Cap()
	if sa.slice.Len()+toAppend.Len() > cap && sa.thread != nil {
		// Consider up to twice the size for the allocation overshoot
		allocation := uintptr((sa.slice.Len()+toAppend.Len())*2 - cap)
		if err := sa.thread.CheckAllocs(int64(allocation * sa.elemType.Size())); err != nil {
			return err
		}
	}
	slice := reflect.AppendSlice(sa.slice, toAppend)
	if slice.Cap() != cap && sa.thread != nil {
		oldSize := int64(roundAllocSize(int64(cap) * int64(sa.elemType.Size())))
		newSize := int64(roundAllocSize(int64(slice.Cap()) * int64(sa.elemType.Size())))
		delta := newSize - oldSize
		sa.allocs += int64(delta)
		if err := sa.thread.AddAllocs(delta); err != nil {
			return err
		}
	}
	sa.slice.Set(slice)
	return nil
}
