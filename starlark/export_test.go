package starlark

import (
	"fmt"
)

func ThreadSafety(thread *Thread) SafetyFlags {
	return thread.requiredSafety
}

const Safe = safetyFlagsLimit - 1

const SafetyFlagsLimit = safetyFlagsLimit

var UniverseSafeties = &universeSafeties

var BytesMethods = bytesMethods
var BytesMethodSafeties = bytesMethodSafeties

var DictMethods = dictMethods
var DictMethodSafeties = dictMethodSafeties

var ListMethods = listMethods
var ListMethodSafeties = listMethodSafeties

var StringMethods = stringMethods
var StringMethodSafeties = stringMethodSafeties

var SetMethods = setMethods
var SetMethodSafeties = setMethodSafeties

type stackFrame struct {
	storage []Value
	frame   *frame
}

var _ Value = stackFrame{}

func (stackFrame) Freeze()        {}
func (stackFrame) String() string { return "stackFrame" }
func (stackFrame) Type() string   { return "stackFrame" }
func (stackFrame) Truth() Bool    { return False }

func (stackFrame) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: stackFrame") }

func StackFrame(thread *Thread, b *Builtin, args Tuple, kwargs []Tuple) (Value, error) {
	frame := thread.frameAt(1)
	result := Value(stackFrame{
		storage: frame.locals,
		frame:   frame,
	})

	if err := thread.AddAllocs(EstimateSize(stackFrame{})); err != nil {
		return nil, err
	}

	return result, nil
}
