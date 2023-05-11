package starlark

import (
	"fmt"
	"reflect"
)

type SafeAppender struct {
	thread   *Thread
	slice    reflect.Value
	elemSize uintptr
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
	return &SafeAppender{
		thread:   thread,
		slice:    slice,
		elemSize: slice.Type().Elem().Size(),
	}
}

func (sa *SafeAppender) Append(values ...interface{}) error {
	cap := sa.slice.Cap()
	if sa.slice.Len()+len(values) > cap {
		if err := sa.thread.CheckAllocs(int64(uintptr(cap)*sa.elemSize) / 5); err != nil {
			return err
		}
	}
	var slice = sa.slice
	for _, value := range values {
		slice = reflect.Append(slice, reflect.ValueOf(value))
	}
	sa.slice.Set(slice)
	delta := roundAllocSize(uintptr(slice.Cap()-cap) * sa.elemSize)
	if delta > 0 {
		if err := sa.thread.AddAllocs(int64(delta)); err != nil {
			return err
		}
	}
	return nil
}
