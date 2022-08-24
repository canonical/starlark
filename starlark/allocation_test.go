package starlark_test

import (
	"math"
	"testing"

	"github.com/canonical/starlark/starlark"
)

func TestPositiveDeltaDeclaration(t *testing.T) {
	const intendedAllocIncrease = 1000

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	// Accept and correctly store reasonable size increase
	allocs0 := thread.Allocs()
	if err := thread.AddAllocs(intendedAllocIncrease); err != nil {
		t.Errorf("Unexpected cancellation: %v", err)
	}
	delta := thread.Allocs() - allocs0
	if delta != intendedAllocIncrease {
		t.Errorf("Incorrect size increase: expected %d but got %d", intendedAllocIncrease, delta)
	}
}

func TestPositiveDeltaDeclarationExceedingMax(t *testing.T) {
	const allocationIncrease = 1000
	const maxAllocs = allocationIncrease / 2

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(maxAllocs)

	// Error when too much memory is required
	if err := thread.AddAllocs(allocationIncrease); err == nil {
		t.Errorf("Expected allocation failure!")
	}
}

func TestOverflowingPositiveDeltaDeclaration(t *testing.T) {
	const allocationIncrease = math.MaxInt64
	const expectedErrMsg = "exceeded memory allocation limits"

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	// Increase so that the next allocation will cause an overflow
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("Unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(allocationIncrease); err != nil {
		t.Errorf("Unexpected error when declaring allocation increase: %v", err)
	}

	// Check overflow detected
	if err := thread.AddAllocs(allocationIncrease); err == nil {
		t.Errorf("Expected allocation increase which would cause an overflow to error")
	} else if errMsg := err.Error(); errMsg != expectedErrMsg {
		t.Errorf("Unexpected error when declaring large allocation increase: expected '%s' but got '%v'", expectedErrMsg, errMsg)
	}
}

func TestNegativeDeltaDeclaration(t *testing.T) {
	const allocGreatest = 1000
	const allocReduction = 100
	const expectedFinalAllocs = allocGreatest - allocReduction

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	if err := thread.AddAllocs(allocGreatest); err != nil {
		t.Errorf("Unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(-allocReduction); err != nil {
		t.Errorf("Unexpected error when declaring allocation reduction: %v", err)
	}
	if allocs := thread.Allocs(); allocs != expectedFinalAllocs {
		t.Errorf("Increase and reduction of allocations lead to incorrect value: expected %v but got %v", expectedFinalAllocs, allocs)
	}
}

func TestOverzealousNegativeDeltaDeclaration(t *testing.T) {
	const allocGreatest = 1000
	const allocReduction = 2 * allocGreatest
	const expectedFinalAllocs = 0

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	if err := thread.AddAllocs(allocGreatest); err != nil {
		t.Errorf("Unexpected error when declaring allocation increase: %v", err)
	}
	if err := thread.AddAllocs(-allocReduction); err != nil {
		t.Errorf("Unexpected error when declaring allocation reduction: %v", err)
	}
	if allocs := thread.Allocs(); allocs != 0 {
		t.Errorf("Expected overzealous alloc reduction to cap allocations at zero: recorded %d allocs instead", allocs)
	}
}

func TestConcurrentAddAllocUsage(t *testing.T) {
	const expectedAllocs = 1_000_000

	thread := new(starlark.Thread)
	thread.SetMaxAllocs(0)

	done := make(chan struct{}, 2)

	callAddAlloc := func(n uint) {
		for i := uint(0); i < n; i++ {
			thread.AddAllocs(1)
		}
		done <- struct{}{}
	}

	go callAddAlloc(expectedAllocs / 2)
	go callAddAlloc(expectedAllocs / 2)

	// Await completion
	totDone := 0
	for totDone != 2 {
		select {
		case <-done:
			totDone++
		}
	}

	if allocs := thread.Allocs(); allocs != expectedAllocs {
		t.Errorf("Concurrent thread.AddAlloc contains a race, expected %d allocs recorded but got %d", expectedAllocs, allocs)
	}
}
