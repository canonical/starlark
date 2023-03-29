package math_test

import (
	"math"
	"testing"

	starlarkmath "github.com/canonical/starlark/lib/math"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestModuleSafeties(t *testing.T) {
	for name, value := range starlarkmath.Module.Members {
		builtin, ok := value.(*starlark.Builtin)
		if !ok {
			continue
		}

		if safety, ok := (*starlarkmath.Safeties)[name]; !ok {
			t.Errorf("builtin math.%s has no safety declaration", name)
		} else if actual := builtin.Safety(); actual != safety {
			t.Errorf("builtin math.%s has incorrect safety: expected %v but got %v", name, safety, actual)
		}
	}

	for name, _ := range *starlarkmath.Safeties {
		if _, ok := starlarkmath.Module.Members[name]; !ok {
			t.Errorf("safety declared for non-existent builtin math.%s", name)
		}
	}
}

func testUnary(t *testing.T, name string, inputs []float64) {
	builtin, ok := starlarkmath.Module.Members[name]
	if !ok {
		t.Fatalf("no such builtin: %s", name)
	}

	for _, input := range inputs {
		st := startest.From(t)

		st.RequireSafety(starlark.MemSafe)

		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{starlark.Float(input)}

				result, err := starlark.Call(thread, builtin, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	}
}

func TestMathCeilAllocs(t *testing.T) {
}

func TestMathCopysignAllocs(t *testing.T) {
}

func TestMathFabsAllocs(t *testing.T) {
	testUnary(t, "fabs", []float64{0, 1, -1, 1 << 60, -1 << 60})
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
	testUnary(t, "round", []float64{0, 0.5, 1})
}

func TestMathExpAllocs(t *testing.T) {
	testUnary(t, "exp", []float64{0, 0.5, 1, -1})
}

func TestMathSqrtAllocs(t *testing.T) {
	testUnary(t, "sqrt", []float64{0, 1, 25})
}

func TestMathAcosAllocs(t *testing.T) {
	testUnary(t, "acos", []float64{0, 1, -0.5})
}

func TestMathAsinAllocs(t *testing.T) {
	testUnary(t, "asin", []float64{0, 1, -0.5})
}

func TestMathAtanAllocs(t *testing.T) {
	testUnary(t, "atan", []float64{0, 1, -100})
}

func TestMathAtan2Allocs(t *testing.T) {
}

func TestMathCosAllocs(t *testing.T) {
	testUnary(t, "cos", []float64{0, math.Pi, -math.Pi / 2})
}

func TestMathHypotAllocs(t *testing.T) {
}

func TestMathSinAllocs(t *testing.T) {
	testUnary(t, "sin", []float64{0, math.Pi, -math.Pi / 2})
}

func TestMathTanAllocs(t *testing.T) {
	testUnary(t, "tan", []float64{0, math.Pi / 2, -math.Pi / 2})
}

func TestMathDegreesAllocs(t *testing.T) {
	testUnary(t, "degrees", []float64{0, math.Pi, -math.Pi})
}

func TestMathRadiansAllocs(t *testing.T) {
	testUnary(t, "radians", []float64{0, 90, -90})
}

func TestMathAcoshAllocs(t *testing.T) {
	testUnary(t, "acosh", []float64{1, 1 << 60})
}

func TestMathAsinhAllocs(t *testing.T) {
	testUnary(t, "asinh", []float64{0, 1000000000, -1000000000})
}

func TestMathAtanhAllocs(t *testing.T) {
	testUnary(t, "atanh", []float64{0, 0.999, -0.999})
}

func TestMathCoshAllocs(t *testing.T) {
	testUnary(t, "cosh", []float64{0, 100, -100})
}

func TestMathSinhAllocs(t *testing.T) {
	testUnary(t, "sinh", []float64{0, 100, -100})
}

func TestMathTanhAllocs(t *testing.T) {
	testUnary(t, "tanh", []float64{0, 100, -100})
}

func TestMathLogAllocs(t *testing.T) {
}

func TestMathGammaAllocs(t *testing.T) {
	testUnary(t, "gamma", []float64{0, 1, 170})
}
