package starlark

import (
	"fmt"
	"reflect"
)

type SafeAppender struct {
	thread   *Thread
	slice    reflect.Value
	elemType reflect.Type
}

func NewSafeAppender(thread *Thread, slicePtr interface{}) *SafeAppender {
	if slicePtr == nil {
		panic("expected pointer to slice, got nil")
	}
	ptr := reflect.ValueOf(slicePtr)
	if kind := ptr.Kind(); kind != reflect.Ptr {
		panic(fmt.Sprintf("expected pointer to slice, got %v", kind))
	}
	slice := ptr.Elem()
	if kind := slice.Kind(); kind != reflect.Slice {
		panic(fmt.Sprintf("expected pointer to slice, got pointer to %v", kind))
	}

	elemType := slice.Type().Elem()
	return &SafeAppender{
		thread:   thread,
		slice:    slice,
		elemType: elemType,
	}
}

func (sa *SafeAppender) Append(values ...interface{}) error {
	cap := sa.slice.Cap()
	if sa.slice.Len()+len(values) > cap {
		if err := sa.thread.CheckAllocs(int64(uintptr(cap) * sa.elemType.Size())); err != nil {
			return err
		}
	}
	slice := sa.slice
	for _, value := range values {
		if value == nil {
			switch sa.elemType.Kind() {
			case reflect.Chan, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
			default:
				panic("unexpected nil")
			}
			slice = reflect.Append(slice, reflect.Zero(sa.elemType))
		} else {
			slice = reflect.Append(slice, reflect.ValueOf(value))
		}
	}
	if slice.Cap() != cap {
		oldSize := int64(roundAllocSize(uintptr(cap) * sa.elemType.Size()))
		newSize := int64(roundAllocSize(uintptr(slice.Cap()) * sa.elemType.Size()))

		if err := sa.thread.AddAllocs(newSize - oldSize); err != nil {
			return err
		}
	}
	sa.slice.Set(slice)
	return nil
}

func (sa *SafeAppender) AppendSlice(values interface{}) error {
	if values == nil {
		panic("expected slice, got nil")
	}
	toAppend := reflect.ValueOf(values)
	if kind := toAppend.Kind(); kind != reflect.Slice {
		panic(fmt.Sprintf("expected slice, got %v", kind))
	}
	cap := sa.slice.Cap()
	if sa.slice.Len()+toAppend.Len() > cap {
		// Consider up to twice the size for the allocation overshoot
		allocation := uintptr((sa.slice.Len()+toAppend.Len())*2 - cap)
		if err := sa.thread.CheckAllocs(int64(allocation * sa.elemType.Size())); err != nil {
			return err
		}
	}
	slice := reflect.AppendSlice(sa.slice, toAppend)
	if slice.Cap() != cap {
		oldSize := int64(roundAllocSize(uintptr(cap) * sa.elemType.Size()))
		newSize := int64(roundAllocSize(uintptr(slice.Cap()) * sa.elemType.Size()))

		if err := sa.thread.AddAllocs(newSize - oldSize); err != nil {
			return err
		}
	}
	sa.slice.Set(slice)
	return nil
}
