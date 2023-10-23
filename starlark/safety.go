package starlark

import (
	"bytes"
	"errors"
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

var safetyNames = [...]string{
	"NotSafe",
	"CPUSafe",
	"MemSafe",
	"TimeSafe",
	"IOSafe",
}

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
	if count == 0 {
		panic("unreachable")
	} else if count == 1 {
		return buf.String()[1:]
	} else {
		buf.WriteByte(')')
		return buf.String()
	}
}

// CheckValid checks that a given set of safety flags contains only defined
// flags.
func (flags Safety) CheckValid() error {
	if flags >= safetyFlagsLimit {
		return errors.New("internal error: invalid safety flags")
	}
	return nil
}

// A SafetyAware value can report its safety, which can be used by a thread to
// prevent operations which cannot make sufficient safety guarantees.
type SafetyAware interface {
	Safety() Safety
}

var _ SafetyAware = Safety(0)
var _ SafetyAware = new(Function)
var _ SafetyAware = new(Builtin)
var _ SafetyAware = new(rangeIterator)
var _ SafetyAware = new(stringElemsIterator)
var _ SafetyAware = new(stringCodepointsIterator)
var _ SafetyAware = new(bytesIterator)
var _ SafetyAware = new(listIterator)
var _ SafetyAware = new(tupleIterator)
var _ SafetyAware = new(keyIterator)

func (set Safety) Safety() Safety { return set }

// Contains returns whether the provided flags are a subset of this set.
func (set Safety) Contains(subset Safety) bool {
	return subset&^set == 0
}

var ErrSandboxViolation = errors.New("sandbox violation")

type SafetyError struct {
	Missing Safety
}

func (se SafetyError) Error() string {
	return "feature unavailable to the sandbox"
}

func (se SafetyError) Is(err error) bool {
	return err == ErrSandboxViolation
}

// CheckContains returns an error if the provided flags are not a subset of this set.
func (set Safety) CheckContains(subset Safety) error {
	if difference := subset &^ set; difference != 0 {
		return &SafetyError{difference}
	}
	return nil
}

// CheckSafety returns an error if the provided value does not report
// sufficient safety for the given thread. CheckSafety allows a nil thread.
func CheckSafety(thread *Thread, value interface{}) error {
	if thread == nil {
		return nil // A nil thread makes no safety requirements.
	}

	safety := NotSafe
	if value, ok := value.(SafetyAware); ok {
		safety = value.Safety()
	}
	return thread.CheckPermits(safety)
}
