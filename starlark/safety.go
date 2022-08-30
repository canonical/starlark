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
	MemSafe
	TimeSafe
	IOSafe
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
	if f >= safetyFlagsLimit {
		return "(invalid safety flags)"
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

	return fmt.Sprintf("(%s)", strings.Join(flagNames, " | "))
}

func (f SafetyFlags) Valid() bool {
	return f < safetyFlagsLimit
}

type InvalidSafetyError struct {
	InvalidFlags uint
}

func (InvalidSafetyError) Error() string {
	return "invalid safety flags"
}

// CheckValid checks that a given set of safety flags contains only defined
// flags.
func (f SafetyFlags) CheckValid() error {
	if !f.Valid() {
		return &InvalidSafetyError{uint(f &^ safe)}
	}
	return nil
}

type Safety interface {
	Safety() SafetyFlags
}

var _ Safety = new(Function)
var _ Safety = new(Builtin)

// Permits checks that safety required ⊆ safety toCheck
func (required SafetyFlags) Permits(toCheck SafetyFlags) bool {
	return toCheck.Valid() && required&^toCheck == 0
}

type SafetyError struct {
	Missing SafetyFlags
}

func (SafetyError) Error() string {
	return "missing safety flags"
}

// MustPermit checks that safety required ⊆ safety toCheck, and details any
// missing flags missing in an error.
func (required SafetyFlags) MustPermit(toCheck SafetyFlags) error {
	if err := toCheck.CheckValid(); err != nil {
		return err
	}
	if missingFlags := required &^ toCheck; missingFlags != 0 {
		return &SafetyError{missingFlags}
	}
	return nil
}
