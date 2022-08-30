package starlark

import (
	"fmt"
	"strings"
)

// Safety represents a set of constraints on executed code.
type Safety uint

// A valid set of safety flags is any subset of the following defined flags.
const (
	NotSafe Safety = 0
	CPUSafe Safety = 1 << (iota - 1)
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

func (f Safety) String() string {
	if f == NotSafe {
		return "NotSafe"
	}
	if f >= safetyFlagsLimit {
		return "(invalid safety flags)"
	}

	flagNames := make([]string, 0, numFlagBitsDefined)

	tryAppendFlag := func(flag Safety, name string) {
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

type InvalidSafetyError struct {
	InvalidFlags uint
}

func (InvalidSafetyError) Error() string {
	return "invalid safety flags"
}

// CheckValid checks that a given set of safety flags contains only defined
// flags.
func (f Safety) CheckValid() error {
	if f >= safetyFlagsLimit {
		return &InvalidSafetyError{uint(f &^ safe)}
	}
	return nil
}

type SafetyAware interface {
	Safety() Safety
}

var _ SafetyAware = new(Function)
var _ SafetyAware = new(Builtin)

// Permits checks that safety required ⊆ safety toCheck
func (required Safety) Permits(toCheck Safety) bool {
	return toCheck.CheckValid() == nil && required&^toCheck == 0
}

type SafetyError struct {
	Missing Safety
}

func (SafetyError) Error() string {
	return "missing safety flags"
}

// MustPermit checks that safety required ⊆ safety toCheck, and details any
// missing flags missing in an error.
func (required Safety) MustPermit(toCheck Safety) error {
	if err := toCheck.CheckValid(); err != nil {
		return err
	}
	if missingFlags := required &^ toCheck; missingFlags != 0 {
		return &SafetyError{missingFlags}
	}
	return nil
}
