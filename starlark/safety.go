package starlark

import (
	"fmt"
	"reflect"
	"strings"
)

// SafetyFlags represents a set of constraints on executed code.
type SafetyFlags uint

var _ fmt.Formatter = SafetyFlags(0)

// A valid set of safety flags is any subset of the following defined flags.
const (
	CPUSafe SafetyFlags = 1 << iota
	IOSafe
	MemSafe
	TimeSafe
	safetyFlagsLimit
)

func (f SafetyFlags) Format(state fmt.State, verb rune) {
	switch verb {
	case 'd':
		state.Write([]byte(fmt.Sprintf("%d", uint(f))))
	case 'x':
		state.Write([]byte(fmt.Sprintf("%#x", uint(f))))
	case 'X':
		state.Write([]byte(fmt.Sprintf("%#X", uint(f))))
	default:
		state.Write([]byte("{"))
		for i, name := range f.Names() {
			if i > 0 {
				state.Write([]byte(", "))
			}
			state.Write([]byte(name))
		}
		state.Write([]byte("}"))
	}
}

// A pointer to a function of any type.
type function uintptr

// Central map of functions to their declared safety flags.
//
// Note that as closures share the same underlying function, all instances of a
// given closure will implicitly share the same safety flags
var knownSafety map[function]SafetyFlags

var numFlagBitsDefined uintptr

func init() {
	for f := safetyFlagsLimit; f >= 1; f >>= 1 {
		numFlagBitsDefined++
	}
}

// flagName takes a safety flag with a single set bit and returns the name of that flag, otherwise the empty string
func (f SafetyFlags) flagName() (name string) {
	switch f {
	case CPUSafe:
		name = "CPUSafe"
	case IOSafe:
		name = "IOSafe"
	case MemSafe:
		name = "MemSafe"
	case TimeSafe:
		name = "TimeSafe"
	}
	return
}

// Names returns a list of human-readable names of set flags
func (flags SafetyFlags) Names() (names []string) {
	names = make([]string, 0, numFlagBitsDefined)
	for f := SafetyFlags(1); f < safetyFlagsLimit; f <<= 1 {
		if f&flags != 0 {
			names = append(names, f.flagName())
		}
	}
	return
}

// CheckValid checks that a given set of safety flags contains only defined
// flags. If this is not the case, it panics.
func (f SafetyFlags) CheckValid() (err error) {
	if f >= safetyFlagsLimit {
		err = fmt.Errorf("Invalid safety flags: got %x", f)
	}
	return
}

// Safety is a convenience method to get the safety of the function which underlies a
// builtin.
func (b *Builtin) Safety() SafetyFlags {
	return SafetyOf(b)
}

// SafetyOf gets the safety of fn, a function which may be used as
// the CallInternal method in a Callable.
func SafetyOf(c Callable) SafetyFlags {
	if b, ok := c.(*Builtin); ok {
		return SafetyOfBuiltinFunc(b.fn)
	}

	var _ = c.CallInternal
	const callInternalMethodName = "CallInternal"
	if ci, ok := reflect.TypeOf(c).MethodByName(callInternalMethodName); ok {
		return function(ci.Func.Pointer()).Safety()
	}
	panic(fmt.Sprintf("No such method '%s' for starlark.Callable %T. This is a bug, please report it at https://github.com/canonical/starlark/issues", callInternalMethodName, c))
}

// SafetyOfBuiltinFunc gets the safety of fn, a function which may be wrapped
// into a Builtin.
func SafetyOfBuiltinFunc(fn func(*Thread, *Builtin, Tuple, []Tuple) (Value, error)) SafetyFlags {
	return function(reflect.ValueOf(fn).Pointer()).Safety()
}

// Safety gets the safety flags of an arbitrarily-typed function at a given
// location.
func (fn function) Safety() (flags SafetyFlags) {
	if knownSafety != nil {
		flags = knownSafety[fn]
	}
	return
}

// DeclareSafety is a convenience function to declare the safety of the
// function which underlies a builtin. Panics if passed flags are not valid.
func (b *Builtin) DeclareSafety(flags SafetyFlags) error {
	return DeclareSafety(b, flags)
}

// DeclareSafety declares the safety of a callable c. Panics if passed flags
// are not valid.
//
// See DeclareBuiltinFuncSafety for pitfalls of using closures as the function
// which underlies callables
func DeclareSafety(c Callable, flags SafetyFlags) (err error) {
	if b, ok := c.(*Builtin); ok {
		return DeclareBuiltinFuncSafety(b.fn, flags)
	}

	var _ = c.CallInternal
	const callInternalMethodName = "CallInternal"
	if ci, ok := reflect.TypeOf(c).MethodByName(callInternalMethodName); ok {
		err = function(ci.Func.Pointer()).DeclareSafety(flags)
	} else {
		err = fmt.Errorf("No such method '%s' for starlark.Callable %T. This is a bug, please report it at https://github.com/canonical/starlark/issues", callInternalMethodName, c)
	}
	return
}

// DeclareBuiltinFuncSafety declares the safety of fn, a function which may be
// wrapped into a Builtin, as flags. Panics if passed flags are not valid.
//
// The first time this function is called for a given fn, the value flags is
// recorded against it exactly. However, subsequent invocations on the same fn
// will record the intersection of the respective flags argument and those
// already stored.
//
// This redeclaration behaviour is significant on some platforms when several
// callables use the same base closure but with different upvalues to represent
// their underlying go function. In this case, as we store the safety flags
// against the function (and not the function-upvalue pair which forms the
// closure), the the safety declarations may interact. When this occurs, the
// strongest assertion which can be made for a set of closures is the largest
// common subset of their safety flags---their intersection. Hence safety
// assertions around closures may sometimes be weakened, which may lead to
// rejection when the Callable is later called.
//
// If this behaviour proves problematic, it may be circumvented by creating a
// separate function to wrap each instance of the closure and passing these to
// starlark, thereby obscuring the common implementation.
func DeclareBuiltinFuncSafety(fn func(*Thread, *Builtin, Tuple, []Tuple) (Value, error), flags SafetyFlags) error {
	return function(reflect.ValueOf(fn).Pointer()).DeclareSafety(flags)
}

// DeclareSafety declares that the safety of an arbitrarily typed function at a
// given location is flags. Panics of passed flags are not valid.
//
// The first invocation on each fn will store flags as passed. Subsequent
// invocations will intersect flags with the value already stored before
// storing this.
func (fn function) DeclareSafety(flags SafetyFlags) error {
	if err := flags.CheckValid(); err != nil {
		return err
	}

	if knownSafety == nil {
		knownSafety = make(map[function]SafetyFlags)
	}

	if _, ok := knownSafety[fn]; ok {
		// Weaken safety guarantee at redeclaration
		knownSafety[fn] &= flags
	} else {
		knownSafety[fn] = flags
	}
	return nil
}

// Permits checks that safety required ⊆ safety toCheck, and details any
// missing flags missing in an error.
func (required SafetyFlags) Permits(toCheck SafetyFlags) error {
	missingFlags := required &^ toCheck
	if missingFlags != 0 {
		return fmt.Errorf("Missing safety flags: %s", strings.Join(missingFlags.Names(), ", "))
	}
	return nil
}
