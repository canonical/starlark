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

func testBinarySafety(t *testing.T, name string, inputs [][2]float64) {
	builtin, ok := starlarkmath.Module.Members[name]
	if !ok {
		t.Fatalf("no such builtin: math.%s", name)
	}

	for _, input := range inputs {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{starlark.Float(input[0]), starlark.Float(input[1])}
				result, err := starlark.Call(thread, builtin, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	}
}

func testUnarySteps(t *testing.T, name string, inputs []starlark.Value) {
	builtin, ok := starlarkmath.Module.Members[name]
	if !ok {
		t.Fatalf("no such builtin: math.%s", name)
	}

	for _, input := range inputs {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMaxExecutionSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, builtin, starlark.Tuple{input}, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	}
}

func testBinarySteps(t *testing.T, name string, inputs [][2]starlark.Value) {
	builtin, ok := starlarkmath.Module.Members[name]
	if !ok {
		t.Fatalf("no such builtin: math.%s", name)
	}

	for _, input := range inputs {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMaxExecutionSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, builtin, starlark.Tuple{input[0], input[1]}, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	}
}

func TestMathCeilSteps(t *testing.T) {
	testUnarySteps(t, "ceil", []starlark.Value{
		starlark.Float(-1.5),
		starlark.Float(0),
		starlark.Float(1.5),
		starlark.MakeInt(1),
		starlark.MakeInt64(1<<32 + 1),
	})
}

func TestMathCeilAllocs(t *testing.T) {
	testMathRoundingAllocs(t, "ceil")
}

func TestMathCopysignSteps(t *testing.T) {
	testBinarySteps(t, "copysign", [][2]starlark.Value{
		{starlark.Float(1), starlark.Float(1)},
		{starlark.Float(1), starlark.Float(-1)},
		{starlark.MakeInt(1), starlark.Float(-1)},
		{starlark.Float(1), starlark.MakeInt(-1)},
		{starlark.MakeInt(1), starlark.MakeInt(-1)},
	})
}

func TestMathCopysignAllocs(t *testing.T) {
	testBinarySafety(t, "copysign", [][2]float64{{1, 1}, {1, -1}})
}

func TestMathFabsSteps(t *testing.T) {
	testUnarySteps(t, "fabs", []starlark.Value{
		starlark.Float(0),
		starlark.Float(1),
		starlark.Float(-1),
		starlark.Float(1 << 60),
		starlark.Float(-1 << 60),
		starlark.MakeInt(0),
		starlark.MakeInt(1),
		starlark.MakeInt(-1),
		starlark.MakeInt64(1 << 60),
		starlark.MakeInt64(-1 << 60),
	})
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

func TestMathFloorSteps(t *testing.T) {
	testUnarySteps(t, "floor", []starlark.Value{
		starlark.Float(-1.5),
		starlark.Float(0),
		starlark.Float(1.5),
		starlark.MakeInt(1),
		starlark.MakeInt64(1<<32 + 1),
	})
}

func TestMathFloorAllocs(t *testing.T) {
	testMathRoundingAllocs(t, "floor")
}

func TestMathModSteps(t *testing.T) {
	testBinarySteps(t, "mod", [][2]starlark.Value{
		{starlark.Float(5.4), starlark.Float(3)},
		{starlark.Float(1), starlark.Float(0)},
		{starlark.MakeInt(10), starlark.Float(1)},
		{starlark.Float(1), starlark.MakeInt(0)},
		{starlark.MakeInt64(1 << 32), starlark.MakeInt(-1)},
	})
}

func TestMathModAllocs(t *testing.T) {
	testBinarySafety(t, "mod", [][2]float64{{5.4, 3}, {1, 0}})
}

func TestMathPowSteps(t *testing.T) {
	testBinarySteps(t, "pow", [][2]starlark.Value{
		{starlark.Float(2), starlark.Float(32)},
		{starlark.Float(0), starlark.Float(0)},
		{starlark.MakeInt(2), starlark.MakeInt(32)},
	})
}

func TestMathPowAllocs(t *testing.T) {
	testBinarySafety(t, "pow", [][2]float64{{2, 32}, {0, 0}})
}

func TestMathRemainderSteps(t *testing.T) {
	testBinarySteps(t, "remainder", [][2]starlark.Value{
		{starlark.Float(5.4), starlark.Float(3)},
		{starlark.Float(1), starlark.Float(0)},
		{starlark.MakeInt(10), starlark.Float(1)},
		{starlark.Float(1), starlark.MakeInt(0)},
		{starlark.MakeInt64(1 << 32), starlark.MakeInt(-1)},
	})
}

func TestMathRemainderAllocs(t *testing.T) {
	testBinarySafety(t, "remainder", [][2]float64{{5.4, 3}, {1, 0}})
}

func TestMathRoundSteps(t *testing.T) {
	testUnarySteps(t, "round", []starlark.Value{
		starlark.Float(-1.5),
		starlark.Float(0),
		starlark.Float(1.5),
		starlark.MakeInt(1),
		starlark.MakeInt64(1<<32 + 1),
	})
}

func TestMathRoundAllocs(t *testing.T) {
	testUnarySafety(t, "round", []float64{0, 0.5, 1})
}

func TestMathExpSteps(t *testing.T) {
	testUnarySteps(t, "exp", []starlark.Value{
		starlark.Float(0),
		starlark.Float(0.5),
		starlark.Float(1),
		starlark.Float(-1),
		starlark.MakeInt(1),
		starlark.MakeInt(-1),
	})
}

func TestMathExpAllocs(t *testing.T) {
	testUnarySafety(t, "exp", []float64{0, 0.5, 1, -1})
}

func TestMathSqrtSteps(t *testing.T) {
	testUnarySteps(t, "exp", []starlark.Value{
		starlark.Float(0),
		starlark.Float(0.5),
		starlark.Float(25),
		starlark.MakeInt(1),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathSqrtAllocs(t *testing.T) {
	testUnarySafety(t, "sqrt", []float64{0, 1, 25})
}

func TestMathAcosSteps(t *testing.T) {
	testUnarySteps(t, "acos", []starlark.Value{
		starlark.Float(0),
		starlark.Float(1),
		starlark.Float(-0.5),
		starlark.MakeInt(1),
	})
}

func TestMathAcosAllocs(t *testing.T) {
	testUnarySafety(t, "acos", []float64{0, 1, -0.5})
}

func TestMathAsinSteps(t *testing.T) {
	testUnarySteps(t, "asin", []starlark.Value{
		starlark.Float(0),
		starlark.Float(1),
		starlark.Float(-0.5),
		starlark.MakeInt(1),
	})
}

func TestMathAsinAllocs(t *testing.T) {
	testUnarySafety(t, "asin", []float64{0, 1, -0.5})
}

func TestMathAtanSteps(t *testing.T) {
	testUnarySteps(t, "atan", []starlark.Value{
		starlark.Float(0),
		starlark.Float(0.5),
		starlark.Float(1),
		starlark.Float(100),
		starlark.MakeInt(1),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathAtanAllocs(t *testing.T) {
	testUnarySafety(t, "atan", []float64{0, 1, -100})
}

func TestMathAtan2Steps(t *testing.T) {
	testBinarySteps(t, "atan2", [][2]starlark.Value{
		{starlark.Float(5.4), starlark.Float(3)},
		{starlark.Float(1), starlark.Float(0)},
		{starlark.MakeInt(10), starlark.Float(1)},
		{starlark.Float(1), starlark.MakeInt(0)},
		{starlark.MakeInt64(1 << 32), starlark.MakeInt(-1)},
	})
}

func TestMathAtan2Allocs(t *testing.T) {
	testBinarySafety(t, "atan2", [][2]float64{{5.4, 3}, {1, 0}})
}

func TestMathCosSteps(t *testing.T) {
	testUnarySteps(t, "cos", []starlark.Value{
		starlark.Float(0),
		starlark.Float(math.Pi),
		starlark.Float(-math.Pi / 2),
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathCosAllocs(t *testing.T) {
	testUnarySafety(t, "cos", []float64{0, math.Pi, -math.Pi / 2})
}

func TestMathHypotSteps(t *testing.T) {
	testBinarySteps(t, "hypot", [][2]starlark.Value{
		{starlark.Float(4), starlark.Float(3)},
		{starlark.Float(1), starlark.Float(0)},
		{starlark.MakeInt(10), starlark.Float(1)},
		{starlark.Float(1), starlark.MakeInt(0)},
		{starlark.MakeInt64(1 << 32), starlark.MakeInt(-1)},
	})
}

func TestMathHypotAllocs(t *testing.T) {
	testBinarySafety(t, "hypot", [][2]float64{{4, 3}, {1, 0}})
}

func TestMathSinSteps(t *testing.T) {
	testUnarySteps(t, "sin", []starlark.Value{
		starlark.Float(0),
		starlark.Float(math.Pi),
		starlark.Float(-math.Pi / 2),
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathSinAllocs(t *testing.T) {
	testUnarySafety(t, "sin", []float64{0, math.Pi, -math.Pi / 2})
}

func TestMathTanSteps(t *testing.T) {
	testUnarySteps(t, "tan", []starlark.Value{
		starlark.Float(0),
		starlark.Float(math.Pi / 2),
		starlark.Float(-math.Pi / 2),
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathTanAllocs(t *testing.T) {
	testUnarySafety(t, "tan", []float64{0, math.Pi / 2, -math.Pi / 2})
}

func TestMathDegreesSteps(t *testing.T) {
	testUnarySteps(t, "degrees", []starlark.Value{
		starlark.Float(0),
		starlark.Float(math.Pi),
		starlark.Float(-math.Pi),
		starlark.MakeInt(0),
		starlark.MakeInt(1),
	})
}

func TestMathDegreesAllocs(t *testing.T) {
	testUnarySafety(t, "degrees", []float64{0, math.Pi, -math.Pi})
}

func TestMathRadiansSteps(t *testing.T) {
	testUnarySteps(t, "radians", []starlark.Value{
		starlark.Float(0),
		starlark.Float(90),
		starlark.Float(-90),
		starlark.MakeInt(0),
	})
}

func TestMathRadiansAllocs(t *testing.T) {
	testUnarySafety(t, "radians", []float64{0, 90, -90})
}

func TestMathAcoshSteps(t *testing.T) {
	testUnarySteps(t, "acosh", []starlark.Value{
		starlark.Float(0),
		starlark.Float(math.Pi),
		starlark.Float(-math.Pi),
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathAcoshAllocs(t *testing.T) {
	testUnarySafety(t, "acosh", []float64{1, 1 << 60})
}

func TestMathAsinhSteps(t *testing.T) {
	testUnarySteps(t, "asinh", []starlark.Value{
		starlark.Float(0),
		starlark.Float(math.Pi),
		starlark.Float(-math.Pi),
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathAsinhAllocs(t *testing.T) {
	testUnarySafety(t, "asinh", []float64{0, 1000000000, -1000000000})
}

func TestMathAtanhSteps(t *testing.T) {
	testUnarySteps(t, "atanh", []starlark.Value{
		starlark.Float(0),
		starlark.Float(0.999),
		starlark.Float(-0.999),
		starlark.MakeInt(0),
	})
}

func TestMathAtanhAllocs(t *testing.T) {
	testUnarySafety(t, "atanh", []float64{0, 0.999, -0.999})
}

func TestMathCoshSteps(t *testing.T) {
	testUnarySteps(t, "cosh", []starlark.Value{
		starlark.Float(0),
		starlark.Float(100),
		starlark.Float(-100),
		starlark.MakeInt(0),
	})
}

func TestMathCoshAllocs(t *testing.T) {
	testUnarySafety(t, "cosh", []float64{0, 100, -100})
}

func TestMathSinhSteps(t *testing.T) {
	testUnarySteps(t, "sinh", []starlark.Value{
		starlark.Float(0),
		starlark.Float(100),
		starlark.Float(-100),
		starlark.MakeInt(0),
	})
}

func TestMathSinhAllocs(t *testing.T) {
	testUnarySafety(t, "sinh", []float64{0, 100, -100})
}

func TestMathTanhSteps(t *testing.T) {
	testUnarySteps(t, "tanh", []starlark.Value{
		starlark.Float(0),
		starlark.Float(100),
		starlark.Float(-100),
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathTanhAllocs(t *testing.T) {
	testUnarySafety(t, "tanh", []float64{0, 100, -100})
}

func TestMathLogSteps(t *testing.T) {
	testUnarySteps(t, "log", []starlark.Value{
		starlark.Float(0),
		starlark.Float(100),
		starlark.Float(-100),
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 32),
	})
}

func TestMathLogAllocs(t *testing.T) {
	testUnarySafety(t, "log", []float64{0, 1, math.E})
	testBinarySafety(t, "log", [][2]float64{{math.E, math.E}, {10000, -10}})
}

func TestMathGammaSteps(t *testing.T) {
	testUnarySteps(t, "gamma", []starlark.Value{
		starlark.Float(0),
		starlark.Float(1),
		starlark.Float(-170),
		starlark.MakeInt(0),
	})
}

func TestMathGammaAllocs(t *testing.T) {
	testUnarySafety(t, "gamma", []float64{0, 1, 170})
}
