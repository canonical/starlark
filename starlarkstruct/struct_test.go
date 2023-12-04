// Copyright 2018 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlarkstruct_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
	"github.com/canonical/starlark/starlarktest"
)

func Test(t *testing.T) {
	testdata := starlarktest.DataFile("starlarkstruct", ".")
	thread := &starlark.Thread{Load: load}
	starlarktest.SetReporter(thread, t)
	filename := filepath.Join(testdata, "testdata/struct.star")
	predeclared := starlark.StringDict{
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"gensym": starlark.NewBuiltin("gensym", gensym),
	}
	if _, err := starlark.ExecFile(thread, filename, nil, predeclared); err != nil {
		if err, ok := err.(*starlark.EvalError); ok {
			t.Fatal(err.Backtrace())
		}
		t.Fatal(err)
	}
}

// load implements the 'load' operation as used in the evaluator tests.
func load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	if module == "assert.star" {
		return starlarktest.LoadAssertModule()
	}
	return nil, fmt.Errorf("load not implemented")
}

// gensym is a built-in function that generates a unique symbol.
func gensym(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("gensym", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return &symbol{name: name}, nil
}

// A symbol is a distinct value that acts as a constructor of "branded"
// struct instances, like a class symbol in Python or a "provider" in Bazel.
type symbol struct{ name string }

var _ starlark.Callable = (*symbol)(nil)

func (sym *symbol) Name() string          { return sym.name }
func (sym *symbol) String() string        { return sym.name }
func (sym *symbol) Type() string          { return "symbol" }
func (sym *symbol) Freeze()               {} // immutable
func (sym *symbol) Truth() starlark.Bool  { return starlark.True }
func (sym *symbol) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: %s", sym.Type()) }

func (sym *symbol) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("%s: unexpected positional arguments", sym)
	}
	return starlarkstruct.FromKeywords(sym, kwargs), nil
}

func TestSafeString(t *testing.T) {
	tests := []struct {
		name  string
		input starlark.SafeStringer
	}{{
		name: "Struct",
		input: starlarkstruct.FromStringDict(
			starlark.String("foo"),
			starlark.StringDict{
				"baz": starlark.None,
			},
		),
	}, {
		name: "Module",
		input: &starlarkstruct.Module{
			Name: "foo",
			Members: starlark.StringDict{
				"baz": starlark.None,
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("nil-thread", func(t *testing.T) {
				builder := new(strings.Builder)
				if err := test.input.SafeString(nil, builder); err != nil {
					t.Errorf("undexpected error: %v", err)
				}
			})

			t.Run("consitency", func(t *testing.T) {
				thread := &starlark.Thread{}
				builder := new(strings.Builder)
				if err := test.input.SafeString(thread, builder); err != nil {
					t.Errorf("undexpected error: %v", err)
				}
				if stringer, ok := test.input.(fmt.Stringer); ok {
					expected := stringer.String()
					actual := builder.String()
					if expected != actual {
						t.Errorf("inconsistent stringer implementation: expected %s got %s", expected, actual)
					}
				}
			})
		})
	}
}
