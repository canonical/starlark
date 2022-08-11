package starlark_test

import (
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestPositiveDeltaDeclaration(t *testing.T) {
	const sizeIncrease = 1000

	thread := new(starlark.Thread)
	thread.SetMaxAllocations(0)

	// Accept and correctly store reasonable size increase
	allocs0 := thread.Allocations()
	err := thread.DeclareSizeIncrease(sizeIncrease)
	if err != nil {
		t.Errorf("Unexpected cancellation: %v", err)
	}
	delta := thread.Allocations() - allocs0
	if delta != sizeIncrease {
		t.Errorf("Incorrect size increase: expected %d but got %d", sizeIncrease, delta)
	}
}

func TestPositiveDeltaDeclarationExceedingQuota(t *testing.T) {
	const sizeIncrease = 1000
	const quota = sizeIncrease / 2

	thread := new(starlark.Thread)
	thread.SetMaxAllocations(quota)

	// Error when too much memory is required
	err := thread.DeclareSizeIncrease(sizeIncrease)
	if err == nil {
		t.Errorf("Expected allocation failure!")
	}
}
