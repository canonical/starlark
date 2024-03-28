// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

// This file defines tests of the Value API.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
	"github.com/canonical/starlark/syntax"
	"github.com/google/go-cmp/cmp"
)

func TestStringMethod(t *testing.T) {
	s := starlark.String("hello")
	for i, test := range [][2]string{
		// quoted string:
		{s.String(), `"hello"`},
		{fmt.Sprintf("%s", s), `"hello"`},
		{fmt.Sprintf("%+s", s), `"hello"`},
		{fmt.Sprintf("%v", s), `"hello"`},
		{fmt.Sprintf("%+v", s), `"hello"`},
		// unquoted:
		{s.GoString(), `hello`},
		{fmt.Sprintf("%#v", s), `hello`},
	} {
		got, want := test[0], test[1]
		if got != want {
			t.Errorf("#%d: got <<%s>>, want <<%s>>", i, got, want)
		}
	}
}

func TestListAppend(t *testing.T) {
	l := starlark.NewList(nil)
	l.Append(starlark.String("hello"))
	res, ok := starlark.AsString(l.Index(0))
	if !ok {
		t.Errorf("failed list.Append() got: %s, want: starlark.String", l.Index(0).Type())
	}
	if res != "hello" {
		t.Errorf("failed list.Append() got: %+v, want: hello", res)
	}
}

func TestParamDefault(t *testing.T) {
	tests := []struct {
		desc         string
		starFn       string
		wantDefaults []starlark.Value
	}{
		{
			desc:         "function with all required params",
			starFn:       "all_required",
			wantDefaults: []starlark.Value{nil, nil, nil},
		},
		{
			desc:   "function with all optional params",
			starFn: "all_opt",
			wantDefaults: []starlark.Value{
				starlark.String("a"),
				starlark.None,
				starlark.String(""),
			},
		},
		{
			desc:   "function with required and optional params",
			starFn: "mix_required_opt",
			wantDefaults: []starlark.Value{
				nil,
				nil,
				starlark.String("c"),
				starlark.String("d"),
			},
		},
		{
			desc:   "function with required, optional, and varargs params",
			starFn: "with_varargs",
			wantDefaults: []starlark.Value{
				nil,
				starlark.String("b"),
				nil,
			},
		},
		{
			desc:   "function with required, optional, varargs, and keyword-only params",
			starFn: "with_varargs_kwonly",
			wantDefaults: []starlark.Value{
				nil,
				starlark.String("b"),
				starlark.String("c"),
				nil,
				nil,
			},
		},
		{
			desc:   "function with required, optional, and keyword-only params",
			starFn: "with_kwonly",
			wantDefaults: []starlark.Value{
				nil,
				starlark.String("b"),
				starlark.String("c"),
				nil,
			},
		},
		{
			desc:   "function with required, optional, and kwargs params",
			starFn: "with_kwargs",
			wantDefaults: []starlark.Value{
				nil,
				starlark.String("b"),
				starlark.String("c"),
				nil,
			},
		},
		{
			desc:   "function with required, optional, varargs, kw-only, and kwargs params",
			starFn: "with_varargs_kwonly_kwargs",
			wantDefaults: []starlark.Value{
				nil,
				starlark.String("b"),
				starlark.String("c"),
				nil,
				starlark.String("e"),
				nil,
				nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			thread := &starlark.Thread{}
			filename := "testdata/function_param.star"
			globals, err := starlark.ExecFile(thread, filename, nil, nil)
			if err != nil {
				t.Fatalf("ExecFile(%v, %q) failed: %v", thread, filename, err)
			}

			fn, ok := globals[tt.starFn].(*starlark.Function)
			if !ok {
				t.Fatalf("value %v is not a Starlark function", globals[tt.starFn])
			}

			var paramDefaults []starlark.Value
			for i := 0; i < fn.NumParams(); i++ {
				paramDefaults = append(paramDefaults, fn.ParamDefault(i))
			}
			if diff := cmp.Diff(tt.wantDefaults, paramDefaults); diff != "" {
				t.Errorf("param defaults got diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSafeString(t *testing.T) {
	tests := []struct {
		name  string
		input starlark.SafeStringer
	}{{
		name:  "Bool",
		input: starlark.True,
	}, {
		name: "Builtin",
		input: starlark.NewBuiltin(
			"test",
			func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
				return starlark.None, nil
			}),
	}, {
		name:  "Bytes",
		input: starlark.Bytes("test"),
	}, {
		name:  "Dict",
		input: starlark.NewDict(0),
	}, {
		name:  "Int(small)",
		input: starlark.MakeInt(10),
	}, {
		name:  "Float",
		input: starlark.Float(3.14),
	}, {
		name: "Function",
		input: func() *starlark.Function {
			const name = "test"
			const expr = "True"
			f, err := starlark.ExprFuncOptions(&syntax.FileOptions{}, name, expr, nil)
			if err != nil {
				t.Fatal(err)
			}
			return f
		}(),
	}, {
		name:  "Int(big)",
		input: starlark.MakeInt64(1 << 32),
	}, {
		name:  "None",
		input: starlark.None,
	}, {
		name:  "Set",
		input: starlark.NewSet(0),
	}, {
		name:  "String",
		input: starlark.String("test"),
	}, {
		name:  "StringDict",
		input: starlark.StringDict{"none": starlark.None},
	}, {
		name:  "Tuple",
		input: starlark.Tuple{starlark.None},
	}, {
		name:  "Bytes iterable",
		input: starlark.Bytes("test").Iterable().(starlark.SafeStringer),
	}, {
		name:  "Range",
		input: starlark.Range(0, 10, 1).(starlark.SafeStringer),
	}, {
		name:  "String elems(chars)",
		input: starlark.String("test").Elems(false).(starlark.SafeStringer),
	}, {
		name:  "String elems(ords)",
		input: starlark.String("test").Elems(true).(starlark.SafeStringer),
	}, {
		name:  "String codepoints(chars)",
		input: starlark.String("test").Codepoints(false).(starlark.SafeStringer),
	}, {
		name:  "String codepoints(ords)",
		input: starlark.String("test").Codepoints(true).(starlark.SafeStringer),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("nil-thread", func(t *testing.T) {
				builder := new(strings.Builder)
				if err := test.input.SafeString(nil, builder); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})

			t.Run("consistency", func(t *testing.T) {
				thread := &starlark.Thread{}
				builder := new(strings.Builder)
				if err := test.input.SafeString(thread, builder); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// At least for builtin variables, the result should be the
				// same regardless of the safety of the context.
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

func TestSafeUnary(t *testing.T) {
	makeInt := func(thread *starlark.Thread, n int) (starlark.Value, error) {
		num := starlark.Value(starlark.MakeInt(1).Lsh(uint(n)))
		if err := thread.AddAllocs(starlark.EstimateSize(num)); err != nil {
			return nil, err
		}
		return num, nil
	}
	makeFloat := func(thread *starlark.Thread, n int) (starlark.Value, error) {
		num := starlark.Value(starlark.Float(n) * starlark.Float(n))
		if err := thread.AddAllocs(starlark.EstimateSize(num)); err != nil {
			return nil, err
		}
		return num, nil
	}
	tests := []struct {
		name  string
		input func(thread *starlark.Thread, n int) (starlark.Value, error)
		op    syntax.Token
		steps uint64
	}{{
		name:  "+Int",
		input: makeInt,
		op:    syntax.PLUS,
		steps: 0,
	}, {
		name:  "-Int",
		input: makeInt,
		op:    syntax.MINUS,
		steps: 1,
	}, {
		name:  "~Int",
		input: makeInt,
		op:    syntax.TILDE,
		steps: 1,
	}, {
		name:  "+Float",
		input: makeFloat,
		op:    syntax.PLUS,
		steps: 0,
	}, {
		name:  "-Float",
		input: makeFloat,
		op:    syntax.MINUS,
		steps: 0,
	}, {
		name:  "~Float",
		input: makeFloat,
		op:    syntax.TILDE,
		steps: 0,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("execution", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.Safe)
				st.SetMaxSteps(test.steps)
				st.RunThread(func(thread *starlark.Thread) {
					input, err := test.input(thread, st.N)
					if err != nil {
						st.Error(err)
					}
					result, err := starlark.SafeUnary(thread, test.op, input)
					if err != nil {
						st.Error(err)
					}
					st.KeepAlive(result)
				})
			})

			if test.steps != 0 {
				t.Run("cancellation", func(t *testing.T) {
					st := startest.From(t)
					st.RequireSafety(starlark.TimeSafe)
					st.SetMaxSteps(0)
					st.RunThread(func(thread *starlark.Thread) {
						thread.Cancel("done")
						input, err := test.input(thread, st.N)
						if err != nil {
							st.Error(err)
						}
						_, err = starlark.SafeUnary(thread, test.op, input)
						if err == nil {
							if thread.Steps() > 0 {
								st.Error("expected cancellation")
							}
						} else if !isStarlarkCancellation(err) {
							st.Errorf("expected cancellation, got: %v", err)
						}
					})
				})
			}
		})
	}
}
