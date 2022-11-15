package math_test

import (
	"testing"

	"github.com/canonical/starlark/lib/math"
	"github.com/canonical/starlark/starlark"
)

func TestModuleSafeties(t *testing.T) {
	for name, value := range math.Module.Members {
		builtin, ok := value.(*starlark.Builtin)
		if !ok {
			continue
		}

		if safety, ok := (*math.Safeties)[name]; !ok {
			t.Errorf("builtin math.%s has no safety declaration", name)
		} else if actual := builtin.Safety(); actual != safety {
			t.Errorf("builtin math.%s has incorrect safety: expected %v but got %v", name, safety, actual)
		}
	}

	for name, _ := range *math.Safeties {
		if _, ok := math.Module.Members[name]; !ok {
			t.Errorf("safety declared for non-existent builtin math.%s", name)
		}
	}
}

func TestMathCeilAllocs(t *testing.T) {
}

func TestMathCopysignAllocs(t *testing.T) {
}

func TestMathFabsAllocs(t *testing.T) {
}

func TestMathFloorAllocs(t *testing.T) {
}

func TestMathModAllocs(t *testing.T) {
}

func TestMathPowAllocs(t *testing.T) {
}

func TestMathRemainderAllocs(t *testing.T) {
}

func TestMathRoundAllocs(t *testing.T) {
}

func TestMathExpAllocs(t *testing.T) {
}

func TestMathSqrtAllocs(t *testing.T) {
}

func TestMathAcosAllocs(t *testing.T) {
}

func TestMathAsinAllocs(t *testing.T) {
}

func TestMathAtanAllocs(t *testing.T) {
}

func TestMathAtan2Allocs(t *testing.T) {
}

func TestMathCosAllocs(t *testing.T) {
}

func TestMathHypotAllocs(t *testing.T) {
}

func TestMathSinAllocs(t *testing.T) {
}

func TestMathTanAllocs(t *testing.T) {
}

func TestMathDegreesAllocs(t *testing.T) {
}

func TestMathRadiansAllocs(t *testing.T) {
}

func TestMathAcoshAllocs(t *testing.T) {
}

func TestMathAsinhAllocs(t *testing.T) {
}

func TestMathAtanhAllocs(t *testing.T) {
}

func TestMathCoshAllocs(t *testing.T) {
}

func TestMathSinhAllocs(t *testing.T) {
}

func TestMathTanhAllocs(t *testing.T) {
}

func TestMathLogAllocs(t *testing.T) {
}

func TestMathGammaAllocs(t *testing.T) {
}
