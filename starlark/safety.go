package starlark

import (
	"bytes"
	"fmt"
	"math/bits"
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

var safetyNames = [...]string{"NotSafe", "CPUSafe", "MemSafe", "TimeSafe", "IOSafe"}

const safe = safetyFlagsLimit - 1

var numSafetyFlagBitsDefined uint

func init() {
	for f := safetyFlagsLimit; f >= 1; f >>= 1 {
		numSafetyFlagBitsDefined++
	}
}

func (flags Safety) String() string {
	if flags == NotSafe {
		return safetyNames[0]
	}

	buf := bytes.Buffer{}
	buf.WriteByte('(')
	count := 0
	for i := 0; i < bits.UintSize; i++ {
		flag := Safety(1 << i)
		if flag > flags {
			break
		}
		if flag&flags == 0 {
			continue
		}
		count++
		if count > 1 {
			buf.WriteByte('|')
		}
		var name string
		if int(i+1) < len(safetyNames) {
			name = safetyNames[i+1]
		} else {
			name = fmt.Sprintf("InvalidSafe(%d)", flag)
		}
		buf.WriteString(name)
	}
	if count == 1 {
		return buf.String()[1:]
	} else {
		buf.WriteByte(')')
		return buf.String()
	}
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
