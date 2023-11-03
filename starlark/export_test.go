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

type StackFrameCapture struct {
	locals []Value
	frame  *frame
}

var _ Value = StackFrameCapture{}

func (sfc StackFrameCapture) Freeze()        {}
func (sfc StackFrameCapture) String() string { return "StackFrameCapture" }
func (sfc StackFrameCapture) Type() string   { return "StackFrameCapture" }
func (sfc StackFrameCapture) Truth() Bool    { return False }

func (sfc StackFrameCapture) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable: StackFrameCapture")
}

// FrameAt return a value representing the memory used
// by the stack frame at the given depth.
func (thread *Thread) FrameAt(depth int) StackFrameCapture {
	frame := thread.frameAt(depth)
	return StackFrameCapture{
		locals: frame.locals,
		frame:  frame,
	}
}
