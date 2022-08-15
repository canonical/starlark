package starlark

import (
	"fmt"
	"strings"
)

// SafetyFlags represents a set of constraints on executed code
type SafetyFlags uint8

//go:generate stringer -type=SafetyFlags
const (
	CPUSafe SafetyFlags = 1 << iota
	IOSafe
	MemSafe
	TimeSafe
	safetyFlagsLimit
)

var safetyAll SafetyFlags

func init() {
	var flag SafetyFlags
	for flag = 1; flag < safetyFlagsLimit; flag <<= 1 {
		safetyAll |= flag
	}
}

var numFlagsDefined uintptr

type HasSafety interface {
	Safety() SafetyFlags
}

var (
	_ HasSafety = (*Thread)(nil)
	_ HasSafety = (*Builtin)(nil)
	_ HasSafety = (*Function)(nil)
)

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
		panic(fmt.Sprintf("Invalid safety flags: got %d", f))
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
