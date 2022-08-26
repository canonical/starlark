package starlark

import (
	"fmt"
	"strings"
)

// SafetyFlags represents a set of constraints on executed code.
type SafetyFlags uint

// A valid set of safety flags is any subset of the following defined flags.
const (
	NotSafe SafetyFlags = 0
	CPUSafe SafetyFlags = 1 << (iota - 1)
	IOSafe
	MemSafe
	TimeSafe
	safetyFlagsLimit
)

const safe = safetyFlagsLimit - 1

var numFlagBitsDefined uint

func init() {
	for f := safetyFlagsLimit; f >= 1; f >>= 1 {
		numFlagBitsDefined++
	}
}

func (f SafetyFlags) String() string {
	if f == NotSafe {
		return "NotSafe"
	}

	flagNames := make([]string, 0, numFlagBitsDefined)

	tryAppendFlag := func(flag SafetyFlags, name string) {
		if f&flag != 0 {
			flagNames = append(flagNames, name)
		}
	}

	tryAppendFlag(CPUSafe, "CPUSafe")
	tryAppendFlag(IOSafe, "IOSafe")
	tryAppendFlag(MemSafe, "MemSafe")
	tryAppendFlag(TimeSafe, "TimeSafe")

	if len(flagNames) == 1 {
		return flagNames[0]
	}

	return fmt.Sprintf("(%s)", strings.Join(flagNames, "|"))
}

func (f SafetyFlags) Valid() bool {
	return f < safetyFlagsLimit
}

// MustBeValid checks that a given set of safety flags contains only defined
// flags. If this is not the case, it panics.
func (f SafetyFlags) MustBeValid() (err error) {
	if !f.Valid() {
		err = fmt.Errorf("Invalid safety flags: got %x", f)
	}
	return
}

type Safety interface {
	Safety() SafetyFlags
}

var _ Safety = new(Function)
var _ Safety = new(Builtin)

// Permits checks that safety required ⊆ safety toCheck
func (required SafetyFlags) Permits(toCheck SafetyFlags) bool {
	return required&^toCheck == 0
}

// MustPermit checks that safety required ⊆ safety toCheck, and details any
// missing flags missing in an error.
func (required SafetyFlags) MustPermit(toCheck SafetyFlags) error {
	if missingFlags := required &^ toCheck; missingFlags != 0 {
		return fmt.Errorf("missing safety flags: %s", missingFlags.String())
	}
	return nil
}
