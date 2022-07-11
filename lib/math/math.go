// Copyright 2021 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package math provides basic constants and mathematical functions.
package math // import "go.starlark.net/lib/math"

import (
	"errors"
	"fmt"
	"math"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const _MATH_COMPLIANCE_DEFAULT = starlark.ComplyMemSafe | starlark.ComplyCPUSafe | starlark.ComplyTimeSafe | starlark.ComplyIOSafe

// Module math is a Starlark module of math-related functions and constants.
// The module defines the following functions:
//
//     ceil(x) - Returns the ceiling of x, the smallest integer greater than or equal to x.
//     copysign(x, y) - Returns a value with the magnitude of x and the sign of y.
//     fabs(x) - Returns the absolute value of x as float.
//     floor(x) - Returns the floor of x, the largest integer less than or equal to x.
//     mod(x, y) - Returns the floating-point remainder of x/y. The magnitude of the result is less than y and its sign agrees with that of x.
//     pow(x, y) - Returns x**y, the base-x exponential of y.
//     remainder(x, y) - Returns the IEEE 754 floating-point remainder of x/y.
//     round(x) - Returns the nearest integer, rounding half away from zero.
//
//     exp(x) - Returns e raised to the power x, where e = 2.718281… is the base of natural logarithms.
//     sqrt(x) - Returns the square root of x.
//
//     acos(x) - Returns the arc cosine of x, in radians.
//     asin(x) - Returns the arc sine of x, in radians.
//     atan(x) - Returns the arc tangent of x, in radians.
//     atan2(y, x) - Returns atan(y / x), in radians.
//                   The result is between -pi and pi.
//                   The vector in the plane from the origin to point (x, y) makes this angle with the positive X axis.
//                   The point of atan2() is that the signs of both inputs are known to it, so it can compute the correct
//                   quadrant for the angle.
//                   For example, atan(1) and atan2(1, 1) are both pi/4, but atan2(-1, -1) is -3*pi/4.
//     cos(x) - Returns the cosine of x, in radians.
//     hypot(x, y) - Returns the Euclidean norm, sqrt(x*x + y*y). This is the length of the vector from the origin to point (x, y).
//     sin(x) - Returns the sine of x, in radians.
//     tan(x) - Returns the tangent of x, in radians.
//
//     degrees(x) - Converts angle x from radians to degrees.
//     radians(x) - Converts angle x from degrees to radians.
//
//     acosh(x) - Returns the inverse hyperbolic cosine of x.
//     asinh(x) - Returns the inverse hyperbolic sine of x.
//     atanh(x) - Returns the inverse hyperbolic tangent of x.
//     cosh(x) - Returns the hyperbolic cosine of x.
//     sinh(x) - Returns the hyperbolic sine of x.
//     tanh(x) - Returns the hyperbolic tangent of x.
//
//     log(x, base) - Returns the logarithm of x in the given base, or natural logarithm by default.
//
//     gamma(x) - Returns the Gamma function of x.
//
// All functions accept both int and float values as arguments.
//
// The module also defines approximations of the following constants:
//
//     e - The base of natural logarithms, approximately 2.71828.
//     pi - The ratio of a circle's circumference to its diameter, approximately 3.14159.
//
var Module = &starlarkstruct.Module{
	Name: "math",
	Members: starlark.StringDict{
		"ceil":      starlark.NewBuiltinComplies("ceil", ceil, _MATH_COMPLIANCE_DEFAULT),
		"copysign":  newBinaryBuiltin("copysign", math.Copysign, _MATH_COMPLIANCE_DEFAULT),
		"fabs":      newUnaryBuiltin("fabs", math.Abs, _MATH_COMPLIANCE_DEFAULT),
		"floor":     starlark.NewBuiltinComplies("floor", floor, _MATH_COMPLIANCE_DEFAULT),
		"mod":       newBinaryBuiltin("round", math.Mod, _MATH_COMPLIANCE_DEFAULT),
		"pow":       newBinaryBuiltin("pow", math.Pow, _MATH_COMPLIANCE_DEFAULT),
		"remainder": newBinaryBuiltin("remainder", math.Remainder, _MATH_COMPLIANCE_DEFAULT),
		"round":     newUnaryBuiltin("round", math.Round, _MATH_COMPLIANCE_DEFAULT),

		"exp":  newUnaryBuiltin("exp", math.Exp, _MATH_COMPLIANCE_DEFAULT),
		"sqrt": newUnaryBuiltin("sqrt", math.Sqrt, _MATH_COMPLIANCE_DEFAULT),

		"acos":  newUnaryBuiltin("acos", math.Acos, _MATH_COMPLIANCE_DEFAULT),
		"asin":  newUnaryBuiltin("asin", math.Asin, _MATH_COMPLIANCE_DEFAULT),
		"atan":  newUnaryBuiltin("atan", math.Atan, _MATH_COMPLIANCE_DEFAULT),
		"atan2": newBinaryBuiltin("atan2", math.Atan2, _MATH_COMPLIANCE_DEFAULT),
		"cos":   newUnaryBuiltin("cos", math.Cos, _MATH_COMPLIANCE_DEFAULT),
		"hypot": newBinaryBuiltin("hypot", math.Hypot, _MATH_COMPLIANCE_DEFAULT),
		"sin":   newUnaryBuiltin("sin", math.Sin, _MATH_COMPLIANCE_DEFAULT),
		"tan":   newUnaryBuiltin("tan", math.Tan, _MATH_COMPLIANCE_DEFAULT),

		"degrees": newUnaryBuiltin("degrees", degrees, _MATH_COMPLIANCE_DEFAULT),
		"radians": newUnaryBuiltin("radians", radians, _MATH_COMPLIANCE_DEFAULT),

		"acosh": newUnaryBuiltin("acosh", math.Acosh, _MATH_COMPLIANCE_DEFAULT),
		"asinh": newUnaryBuiltin("asinh", math.Asinh, _MATH_COMPLIANCE_DEFAULT),
		"atanh": newUnaryBuiltin("atanh", math.Atanh, _MATH_COMPLIANCE_DEFAULT),
		"cosh":  newUnaryBuiltin("cosh", math.Cosh, _MATH_COMPLIANCE_DEFAULT),
		"sinh":  newUnaryBuiltin("sinh", math.Sinh, _MATH_COMPLIANCE_DEFAULT),
		"tanh":  newUnaryBuiltin("tanh", math.Tanh, _MATH_COMPLIANCE_DEFAULT),

		"log": starlark.NewBuiltinComplies("log", log, _MATH_COMPLIANCE_DEFAULT),

		"gamma": newUnaryBuiltin("gamma", math.Gamma, _MATH_COMPLIANCE_DEFAULT),

		"e":  starlark.Float(math.E),
		"pi": starlark.Float(math.Pi),
	},
}

// floatOrInt is an Unpacker that converts a Starlark int or float to Go's float64.
type floatOrInt float64

func (p *floatOrInt) Unpack(v starlark.Value) error {
	switch v := v.(type) {
	case starlark.Int:
		*p = floatOrInt(v.Float())
		return nil
	case starlark.Float:
		*p = floatOrInt(v)
		return nil
	}
	return fmt.Errorf("got %s, want float or int", v.Type())
}

// newUnaryBuiltin wraps a unary floating-point Go function
// as a Starlark built-in that accepts int or float arguments.
func newUnaryBuiltin(name string, fn func(float64) float64, compliance starlark.ComplianceFlags) *starlark.Builtin {
	return starlark.NewBuiltinComplies(name, func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var x floatOrInt
		if err := starlark.UnpackPositionalArgs(name, args, kwargs, 1, &x); err != nil {
			return nil, err
		}
		return starlark.Float(fn(float64(x))), nil
	}, compliance)
}

// newBinaryBuiltin wraps a binary floating-point Go function
// as a Starlark built-in that accepts int or float arguments.
func newBinaryBuiltin(name string, fn func(float64, float64) float64, compliance starlark.ComplianceFlags) *starlark.Builtin {
	return starlark.NewBuiltinComplies(name, func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var x, y floatOrInt
		if err := starlark.UnpackPositionalArgs(name, args, kwargs, 2, &x, &y); err != nil {
			return nil, err
		}
		return starlark.Float(fn(float64(x), float64(y))), nil
	}, compliance)
}

//  log wraps the Log function
// as a Starlark built-in that accepts int or float arguments.
func log(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		x    floatOrInt
		base floatOrInt = math.E
	)
	if err := starlark.UnpackPositionalArgs("log", args, kwargs, 1, &x, &base); err != nil {
		return nil, err
	}
	if base == 1 {
		return nil, errors.New("division by zero")
	}
	return starlark.Float(math.Log(float64(x)) / math.Log(float64(base))), nil
}

func ceil(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var x starlark.Value

	if err := starlark.UnpackPositionalArgs("ceil", args, kwargs, 1, &x); err != nil {
		return nil, err
	}

	switch t := x.(type) {
	case starlark.Int:
		return t, nil
	case starlark.Float:
		return starlark.NumberToInt(starlark.Float(math.Ceil(float64(t))))
	}

	return nil, fmt.Errorf("got %s, want float or int", x.Type())
}

func floor(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var x starlark.Value

	if err := starlark.UnpackPositionalArgs("floor", args, kwargs, 1, &x); err != nil {
		return nil, err
	}

	switch t := x.(type) {
	case starlark.Int:
		return t, nil
	case starlark.Float:
		return starlark.NumberToInt(starlark.Float(math.Floor(float64(t))))
	}

	return nil, fmt.Errorf("got %s, want float or int", x.Type())
}

func degrees(x float64) float64 {
	return 360 * x / (2 * math.Pi)
}

func radians(x float64) float64 {
	return 2 * math.Pi * x / 360
}
