package starlark

import (
	"fmt"
	"strings"
)

// ComplianceFlags represents a set of constraints on executed code
type ComplianceFlags uint8

//go:generate stringer -type=ComplianceFlags
const (
	// Execute only code which requests memory before making allocations
	MemSafe ComplianceFlags = 1 << iota
	CPUSafe
	TimeSafe
	IOSafe
	complianceFlagsLimit
)

var complianceAll ComplianceFlags

func init() {
	var flag ComplianceFlags
	for flag = 1; flag < complianceFlagsLimit; flag <<= 1 {
		complianceAll |= flag
	}
}

var numFlagsDefined uintptr

type HasCompliance interface {
	Compliance() ComplianceFlags
}

var (
	_ HasCompliance = (*Thread)(nil)
	_ HasCompliance = (*Builtin)(nil)
	_ HasCompliance = (*Function)(nil)
)


func (flags ComplianceFlags) Names() (names []string) {
	names = make([]string, 0, numFlagsDefined)
	for f := ComplianceFlags(1); f < complianceFlagsLimit; f <<= 1 {
		if f&flags != 0 {
			names = append(names, f.String())
		}
	}
	return
}

func (f ComplianceFlags) AssertValid() {
	if f >= complianceFlagsLimit {
		panic(fmt.Sprintf("Invalid compliance flags, %d is not less than %d", f, complianceFlagsLimit))
	}
}

// Tests that compliance required ⊆ compliance toCheck
func (required ComplianceFlags) Permits(toCheck ComplianceFlags) error {
	missingFlags := required &^ toCheck
	if missingFlags != 0 {
		return fmt.Errorf("Missing compliance flags: %s", strings.Join(missingFlags.Names(), ", "))
	}
	return nil
}
