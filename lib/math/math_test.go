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

func testUnarySafety(t *testing.T, name string, inputs []float64) {
	builtin, ok := starlarkmath.Module.Members[name]
	if !ok {
		t.Fatalf("no such builtin: math.%s", name)
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
	testMathRoundingAllocs(t, "ceil")
}

func TestMathCopysignAllocs(t *testing.T) {
}

func TestMathFabsAllocs(t *testing.T) {
	testUnarySafety(t, "fabs", []float64{0, 1, -1, 1 << 60, -1 << 60})
}

func testMathRoundingAllocs(t *testing.T, name string) {
	fn, ok := starlarkmath.Module.Members[name]
	if !ok {
		t.Fatalf("no such builtin: math.%s", name)
	}

	t.Run("type=float", func(t *testing.T) {
		testUnarySafety(t, name, []float64{-1.5, 0, 1.5})
	})

	t.Run("type=int", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.MakeInt(100)}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, fn, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("type=big-int", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			big := starlark.Value(starlark.MakeInt64(1<<32 + 1))
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, fn, starlark.Tuple{big}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestMathFloorAllocs(t *testing.T) {
	testMathRoundingAllocs(t, "floor")
}

func TestMathModAllocs(t *testing.T) {
}

func TestMathPowAllocs(t *testing.T) {
}

func TestMathRemainderAllocs(t *testing.T) {
}

func TestMathRoundAllocs(t *testing.T) {
	testUnarySafety(t, "round", []float64{0, 0.5, 1})
}

func TestMathExpAllocs(t *testing.T) {
	testUnarySafety(t, "exp", []float64{0, 0.5, 1, -1})
}

func TestMathSqrtAllocs(t *testing.T) {
	testUnarySafety(t, "sqrt", []float64{0, 1, 25})
}

func TestMathAcosAllocs(t *testing.T) {
	testUnarySafety(t, "acos", []float64{0, 1, -0.5})
}

func TestMathAsinAllocs(t *testing.T) {
	testUnarySafety(t, "asin", []float64{0, 1, -0.5})
}

func TestMathAtanAllocs(t *testing.T) {
	testUnarySafety(t, "atan", []float64{0, 1, -100})
}

func TestMathAtan2Allocs(t *testing.T) {
}

func TestMathCosAllocs(t *testing.T) {
	testUnarySafety(t, "cos", []float64{0, math.Pi, -math.Pi / 2})
}

func TestMathHypotAllocs(t *testing.T) {
}

func TestMathSinAllocs(t *testing.T) {
	testUnarySafety(t, "sin", []float64{0, math.Pi, -math.Pi / 2})
}

func TestMathTanAllocs(t *testing.T) {
	testUnarySafety(t, "tan", []float64{0, math.Pi / 2, -math.Pi / 2})
}

func TestMathDegreesAllocs(t *testing.T) {
	testUnarySafety(t, "degrees", []float64{0, math.Pi, -math.Pi})
}

func TestMathRadiansAllocs(t *testing.T) {
	testUnarySafety(t, "radians", []float64{0, 90, -90})
}

func TestMathAcoshAllocs(t *testing.T) {
	testUnarySafety(t, "acosh", []float64{1, 1 << 60})
}

func TestMathAsinhAllocs(t *testing.T) {
	testUnarySafety(t, "asinh", []float64{0, 1000000000, -1000000000})
}

func TestMathAtanhAllocs(t *testing.T) {
	testUnarySafety(t, "atanh", []float64{0, 0.999, -0.999})
}

func TestMathCoshAllocs(t *testing.T) {
	testUnarySafety(t, "cosh", []float64{0, 100, -100})
}

func TestMathSinhAllocs(t *testing.T) {
	testUnarySafety(t, "sinh", []float64{0, 100, -100})
}

func TestMathTanhAllocs(t *testing.T) {
	testUnarySafety(t, "tanh", []float64{0, 100, -100})
}

func TestMathLogAllocs(t *testing.T) {
}

func TestMathGammaAllocs(t *testing.T) {
	testUnarySafety(t, "gamma", []float64{0, 1, 170})
}
