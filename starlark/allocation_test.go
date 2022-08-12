package starlark_test

import (
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestPositiveDeltaDeclaration(t *testing.T) {
	const intendedAllocationIncrease = 1000

	thread := new(starlark.Thread)
	thread.SetMaxAllocations(0)

	// Accept and correctly store reasonable size increase
	allocs0 := thread.Allocations()
	if err := thread.DeclareAllocationsIncrease(intendedAllocationIncrease); err != nil {
		t.Errorf("Unexpected cancellation: %v", err)
	}
	delta := thread.Allocations() - allocs0
	if delta != intendedAllocationIncrease {
		t.Errorf("Incorrect size increase: expected %d but got %d", intendedAllocationIncrease, delta)
	}
}

func TestPositiveDeltaDeclarationExceedingMax(t *testing.T) {
	const allocationIncrease = 1000
	const maxAllocations = allocationIncrease / 2

	thread := new(starlark.Thread)
	thread.SetMaxAllocations(maxAllocations)

	// Error when too much memory is required
	if err := thread.DeclareAllocationsIncrease(allocationIncrease); err == nil {
		t.Errorf("Expected allocation failure!")
	}
}
