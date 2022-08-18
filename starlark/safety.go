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

var knownSafety map[uintptr]SafetyFlags


var numFlagsDefined uintptr

func (flags SafetyFlags) Names() (names []string) {
	names = make([]string, 0, numFlagsDefined)
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
	return safetyOf(reflect.ValueOf(fn).Pointer())
}

func SafetyOfBuiltinFunc(fn func(*Thread, *Builtin, Tuple, []Tuple) (Value, error)) SafetyFlags {
	return safetyOf(reflect.ValueOf(fn).Pointer())
}

func safetyOf(fnPtr uintptr) (flags SafetyFlags) {
	if knownSafety != nil {
		flags = knownSafety[fnPtr]
	}
	return
}

func (b *Builtin) DeclareSafety(flags SafetyFlags) {
	DeclareBuiltinFuncSafety(b.fn, flags)
}

func DeclareCallableFuncSafety(fn func(*Thread, Tuple, []Tuple) (Value, error), flags SafetyFlags) {
	setSafety(reflect.ValueOf(fn).Pointer(), flags)
}

func DeclareBuiltinFuncSafety(fn func(*Thread, *Builtin, Tuple, []Tuple) (Value, error), flags SafetyFlags) {
	setSafety(reflect.ValueOf(fn).Pointer(), flags)
}

func setSafety(fnPtr uintptr, flags SafetyFlags) {
	flags.AssertValid()
	if knownSafety == nil {
		knownSafety = make(map[uintptr]SafetyFlags)
	}

	if previousSafety, ok := knownSafety[fnPtr]; ok {
		// Only reduce compliance if attempting to de-declare
		knownSafety[fnPtr] = previousSafety & flags
	} else {
		knownSafety[fnPtr] = flags
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
