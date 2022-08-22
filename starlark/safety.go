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
//
//go:generate stringer -type=SafetyFlags
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

// Names returns a list of human-readable names of set flags
func (flags SafetyFlags) Names() (names []string) {
	names = make([]string, 0, numFlagBitsDefined)
	for f := SafetyFlags(1); f < safetyFlagsLimit; f <<= 1 {
		if f&flags != 0 {
			names = append(names, f.String())
		}
	}
	return
}

// AssertValid checks that a given set of safety flags contains only defined
// flags. If this is not the case, it panics.
func (f SafetyFlags) AssertValid() {
	if f >= safetyFlagsLimit {
		panic(fmt.Sprintf("Invalid safety flags: got %x", f))
	}
}

// Safety is a convenience function to get the safety of the function which underlies a
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

	const callInternalMethodName = "CallInternal"
	if ci, ok := reflect.TypeOf(c).MethodByName(callInternalMethodName); ok {
		return function(ci.Func.Pointer()).Safety()
	}
	panic(fmt.Sprintf("No such method '%s' for callable %v", callInternalMethodName, c))
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

// Convenience function to declare the safety of the function which underlies a
// builtin. Panics if passed flags are not valid.
func (b *Builtin) DeclareSafety(flags SafetyFlags) {
	DeclareSafety(b, flags)
}

// Declare the safety of fn, a function which may be used as the CallInternal
// method in a Callable, as flags. Panics if passed flags are not valid.
func DeclareSafety(c Callable, flags SafetyFlags) {
	if b, ok := c.(*Builtin); ok {
		DeclareBuiltinFuncSafety(b.fn, flags)
		return
	}

	const callInternalMethodName = "CallInternal"
	if ci, ok := reflect.TypeOf(c).MethodByName(callInternalMethodName); ok {
		function(ci.Func.Pointer()).DeclareSafety(flags)
	} else {
		panic(fmt.Sprintf("No such method '%s' for callable %v", callInternalMethodName, c))
	}
}

// Declare the safety of fn, a function which may be wrapped into a Builtin, as
// flags. Panics if passed flags are not valid.
func DeclareBuiltinFuncSafety(fn func(*Thread, *Builtin, Tuple, []Tuple) (Value, error), flags SafetyFlags) {
	function(reflect.ValueOf(fn).Pointer()).DeclareSafety(flags)
}

// Declare that the safety of an arbitrarily typed function at a given location
// is flags. Panics of passed flags are not valid.
//
// The first time this function is called for a given function, the value flags
// is recorded against it exactly. Otherwise, on the second and later calls for
// the same function, the intersection of the then-stored safety flags and
// flags is stored.
//
// This is significant in the case where multiple closures with the same base
// are defined. As these are represented by a set of upvalues and a pointer to
// the base function, and as safety is defined against these base functions
// only, safety declarations on different closures may intereact. The strongest
// guarantee available for a set of same-base closures, _C,_ is the
// intersection of all safety flags of the closures in _C,_ which can cause the
// safety guarantees of closures in _C_ to be weakened, which may lead to their
// being rejected at runtime. If problematic, this can sometimes be remedied by
// creating a function _f_ which wraps the closure, declaring safety on _f_ and
// exposing _f_ to Starlark.
func (fn function) DeclareSafety(flags SafetyFlags) {
	flags.AssertValid()
	if knownSafety == nil {
		knownSafety = make(map[function]SafetyFlags)
	}

	if _, ok := knownSafety[fn]; ok {
		// Weaken safety guarantee at redeclaration
		knownSafety[fn] &= flags
	} else {
		knownSafety[fn] = flags
	}
}

// Permits tests that safety required ⊆ safety toCheck, and details any missing
// flags missing in an error.
func (required SafetyFlags) Permits(toCheck SafetyFlags) error {
	missingFlags := required &^ toCheck
	if missingFlags != 0 {
		return fmt.Errorf("Missing safety flags: %s", strings.Join(missingFlags.Names(), ", "))
	}
	return nil
}
