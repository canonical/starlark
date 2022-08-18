package starlark

import (
	"fmt"
	"reflect"
	"strings"
)

// SafetyFlags represents a set of constraints on executed code
type SafetyFlags uint

var _ fmt.Formatter = SafetyFlags(0)

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

type function uintptr

var knownSafety map[function]SafetyFlags

var numFlagBitsDefined uintptr

func init() {
	for f := safetyFlagsLimit; f >= 1; f >>= 1 {
		numFlagBitsDefined++
	}
}

func (flags SafetyFlags) Names() (names []string) {
	names = make([]string, 0, numFlagBitsDefined)
	for f := SafetyFlags(1); f < safetyFlagsLimit; f <<= 1 {
		if f&flags != 0 {
			names = append(names, f.String())
		}
	}
	return
}

func (f SafetyFlags) AssertValid() {
	if f >= safetyFlagsLimit {
		panic(fmt.Sprintf("Invalid safety flags: got %x", f))
	}
}

func (b *Builtin) Safety() SafetyFlags {
	return SafetyOfBuiltinFunc(b.fn)
}

func SafetyOfCallableFunc(fn func(*Thread, Tuple, []Tuple) (Value, error)) SafetyFlags {
	return function(reflect.ValueOf(fn).Pointer()).safety()
}

func SafetyOfBuiltinFunc(fn func(*Thread, *Builtin, Tuple, []Tuple) (Value, error)) SafetyFlags {
	return function(reflect.ValueOf(fn).Pointer()).safety()
}

// Get the safety of an arbitrarily-type function at a given location
func (fn function) safety() (flags SafetyFlags) {
	if knownSafety != nil {
		flags = knownSafety[fn]
	}
	return
}

func (b *Builtin) DeclareSafety(flags SafetyFlags) {
	DeclareBuiltinFuncSafety(b.fn, flags)
}

func DeclareCallableFuncSafety(fn func(*Thread, Tuple, []Tuple) (Value, error), flags SafetyFlags) {
	function(reflect.ValueOf(fn).Pointer()).declareSafety(flags)
}

func DeclareBuiltinFuncSafety(fn func(*Thread, *Builtin, Tuple, []Tuple) (Value, error), flags SafetyFlags) {
	function(reflect.ValueOf(fn).Pointer()).declareSafety(flags)
}

func (fn function) declareSafety(flags SafetyFlags) {
	flags.AssertValid()
	if knownSafety == nil {
		knownSafety = make(map[function]SafetyFlags)
	}

	if _, ok := knownSafety[fn]; ok {
		// Only reduce compliance if attempting to de-declare
		knownSafety[fn] &= flags
	} else {
		knownSafety[fn] = flags
	}
}

// Tests that safety required ⊆ safety toCheck
func (required SafetyFlags) Permits(toCheck SafetyFlags) error {
	missingFlags := required &^ toCheck
	if missingFlags != 0 {
		return fmt.Errorf("Missing safety flags: %s", strings.Join(missingFlags.Names(), ", "))
	}
	return nil
}
