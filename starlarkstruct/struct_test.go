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
	"github.com/canonical/starlark/startest"
)

func isStarlarkCancellation(err error) bool {
	return strings.Contains(err.Error(), "Starlark computation cancelled:")
}

var make_struct = starlark.NewBuiltinWithSafety("struct", starlarkstruct.MakeSafety, starlarkstruct.Make)

func Test(t *testing.T) {
	testdata := starlarktest.DataFile("starlarkstruct", ".")
	thread := &starlark.Thread{Load: load}
	starlarktest.SetReporter(thread, t)
	filename := filepath.Join(testdata, "testdata/struct.star")
	predeclared := starlark.StringDict{
		"struct": make_struct,
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

func TestStructSafeAttr(t *testing.T) {
	struct_ := starlarkstruct.FromStringDict(
		starlark.String("foo"),
		starlark.StringDict{
			"bar": starlark.None,
		},
	)

	t.Run("nil-thread", func(t *testing.T) {
		defer func() {
			if err := recover(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()

		_, err := struct_.SafeAttr(nil, "bar")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("consistency", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(starlarkstruct.MakeModuleSafety)

		safeResult, err := struct_.SafeAttr(nil, "bar")
		if err != nil {
			t.Error(err)
		}
		unsafeResult, err := struct_.Attr("bar")
		if err != nil {
			t.Error(err)
		}
		if safeResult != unsafeResult {
			t.Errorf("unconsistent SafeAttr implementation: expected %v and %v to be equal", safeResult, unsafeResult)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.TimeSafe)
		st.SetMaxSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			thread.Cancel("done")
			entries := make([]starlark.Tuple, st.N+2)
			// Worst case is to get the second element which takes ceil(log(N)) comparisons
			entries[0] = starlark.Tuple{starlark.String("a"), starlark.None}
			entries[1] = starlark.Tuple{starlark.String("b"), starlark.None}
			for i := 0; i < st.N; i++ {
				key := fmt.Sprintf("c%d", i)
				entries[i+2] = starlark.Tuple{starlark.String(key), starlark.None}
			}
			struct_ := starlarkstruct.FromKeywords(starlarkstruct.Default, entries)
			_, err := struct_.SafeAttr(thread, "b")
			if err == nil {
				st.Error("expected cancellation")
			} else if !isStarlarkCancellation(err) {
				st.Errorf("expected cancellation, got: %v", err)
			}
		})
	})
}

func TestStructSafeString(t *testing.T) {
	struct_ := starlarkstruct.FromStringDict(
		starlark.String("foo"),
		starlark.StringDict{
			"bar": starlark.None,
		},
	)

	t.Run("nil-thread", func(t *testing.T) {
		defer func() {
			if err := recover(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()

		builder := new(strings.Builder)
		if err := struct_.SafeString(nil, builder); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("consistency", func(t *testing.T) {
		thread := &starlark.Thread{}
		builder := new(strings.Builder)
		if err := struct_.SafeString(thread, builder); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		unsafeResult := struct_.String()
		safeResult := builder.String()
		if unsafeResult != safeResult {
			t.Errorf("inconsistent stringer implementation: expected %s got %s", unsafeResult, safeResult)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.TimeSafe)
		st.SetMaxSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			thread.Cancel("done")
			builder := starlark.NewSafeStringBuilder(thread)
			err := struct_.SafeString(thread, builder)
			if err == nil {
				st.Error("expected cancellation")
			} else if !isStarlarkCancellation(err) {
				st.Errorf("expected cancellation, got: %v", err)
			}
		})
	})
}

func TestFromKeywords(t *testing.T) {
	t.Run("nil-thread", func(t *testing.T) {
		defer func() {
			if err := recover(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()

		kwargs := []starlark.Tuple{
			{starlark.String("foo"), starlark.None},
		}
		_, err := starlarkstruct.SafeFromKeywords(nil, starlarkstruct.Default, kwargs)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("resources", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(1)
		st.SetMaxSteps(1)
		st.RunThread(func(thread *starlark.Thread) {
			pairs := make([][2]starlark.Value, st.N)
			kwargs := make([]starlark.Tuple, st.N)
			for i := 0; i < st.N; i++ {
				key := starlark.String(fmt.Sprintf("%012d", i))
				if err := thread.AddAllocs(starlark.EstimateSizeOld(key)); err != nil {
					st.Error(err)
				}
				pairs[i] = [2]starlark.Value{key, starlark.None}
				kwargs[i] = pairs[i][:]
			}
			result, err := starlarkstruct.SafeFromKeywords(thread, starlarkstruct.Default, kwargs)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestFromStringDict(t *testing.T) {
	t.Run("nil-thread", func(t *testing.T) {
		defer func() {
			if err := recover(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()

		dict := starlark.StringDict{
			"foo": starlark.None,
		}
		_, err := starlarkstruct.SafeFromStringDict(nil, starlarkstruct.Default, dict)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("resources", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMinSteps(1)
		st.SetMaxSteps(1)
		st.RunThread(func(thread *starlark.Thread) {
			d := make(starlark.StringDict, st.N)
			for i := 0; i < st.N; i++ {
				key := fmt.Sprintf("%012d", i)
				if err := thread.AddAllocs(starlark.EstimateSizeOld(key)); err != nil {
					st.Error(err)
				}
				d[key] = starlark.None
			}
			result, err := starlarkstruct.SafeFromStringDict(thread, starlarkstruct.Default, d)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestStructResources(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
	st.SetMinSteps(1)
	st.SetMaxSteps(1)
	st.RunThread(func(thread *starlark.Thread) {
		pairs := make([][2]starlark.Value, st.N)
		kwargs := make([]starlark.Tuple, st.N)
		for i := 0; i < st.N; i++ {
			key := starlark.String(fmt.Sprintf("%012d", i))
			if err := thread.AddAllocs(starlark.EstimateSizeOld(key)); err != nil {
				st.Error(err)
			}
			pairs[i] = [2]starlark.Value{key, starlark.None}
			kwargs[i] = pairs[i][:]
		}
		result, err := starlark.Call(thread, make_struct, nil, kwargs)
		if err != nil {
			st.Error(err)
		}
		st.KeepAlive(result)
	})
}
