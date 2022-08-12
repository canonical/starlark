package starlark

import (
	"fmt"
	"strings"
)

// ComplianceFlags represents a set of constraints on executed code
type ComplianceFlags uint8

const (
	// Execute only code which requests memory before making allocations
	MemSafe ComplianceFlags = 1 << iota
	CPUSafe
	TimeSafe
	IOSafe
	complianceFlagsLimit
)

var complianceAll ComplianceFlags

var complianceFlagNames = map[ComplianceFlags]string{
	MemSafe:  "memsafe",
	CPUSafe:  "cpusafe",
	TimeSafe: "timesafe",
	IOSafe:   "iosafe",
}

var complianceFlagsFromNames = map[string]ComplianceFlags{}

func init() {
	for flag, name := range complianceFlagNames {
		complianceFlagsFromNames[name] = flag
	}
	var flag ComplianceFlags
	for flag = 1; flag < complianceFlagsLimit; flag <<= 1 {
		complianceAll |= flag
	}
}

type HasCompliance interface {
	Compliance() ComplianceFlags
}

var (
	_ HasCompliance = (*Thread)(nil)
	_ HasCompliance = (*Builtin)(nil)
	_ HasCompliance = (*Function)(nil)
)

func (f ComplianceFlags) Names() (names []string) {
	names = make([]string, 0, len(complianceFlagNames))
	for i := ComplianceFlags(1); i < complianceFlagsLimit; i <<= 1 {
		if i&f != 0 {
			names = append(names, complianceFlagNames[i])
		}
	}
	return
}

func ComplianceFromNames(names []string) (f ComplianceFlags, _ error) {
	for _, name := range names {
		if g, ok := complianceFlagsFromNames[name]; ok {
			f |= g
		} else {
			validNames := make([]string, 0, len(complianceFlagsFromNames))
			for validName, _ := range complianceFlagsFromNames {
				validNames = append(validNames, validName)
			}
			return 0, fmt.Errorf("No such compliance flag '%s', expected one of: %s", name, strings.Join(validNames, ", "))
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
