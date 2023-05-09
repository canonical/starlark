package starlark

import (
	"fmt"
	"reflect"
)

type SafeAppender struct {
	thread   *Thread
	slicePtr reflect.Value
	elemSize int64
}

func NewSafeAppender(thread *Thread, slicePtr interface{}) *SafeAppender {
	if slicePtr == nil {
		panic("expected pointer to slice, got nil")
	}
	ptr := reflect.ValueOf(slicePtr)
	if kind := ptr.Kind(); kind != reflect.Ptr {
		panic(fmt.Sprintf("expected pointer to slice, got %v", kind))
	}
	if kind := ptr.Elem().Kind(); kind != reflect.Slice {
		panic(fmt.Sprintf("expected pointer to slice, got pointer to %v", kind))
	}
	return &SafeAppender{
		thread:   thread,
		slicePtr: ptr,
		elemSize: EstimateSize(reflect.New(ptr.Type().Elem())), // TODO(kcza): avoid allocation spike here? (eg. could make new [1000]int)
	}
}

func (sa *SafeAppender) Append(values ...interface{}) error {
	slice := sa.slicePtr.Elem()
	cap := slice.Cap()
	if slice.Len()+len(values) > cap {
		if err := sa.thread.CheckAllocs(int64(cap)); err != nil {
			return err
		}
	}
	for _, value := range values {
		slice = reflect.Append(slice, reflect.ValueOf(value))
	}
	sa.slicePtr.Elem().Set(slice)
	delta := int64(slice.Cap() - cap)
	if delta > 0 { // TODO(kcza): account for size-classes
		if err := sa.thread.AddAllocs(delta * sa.elemSize); err != nil {
			return err
		}
	}
	return nil
}
