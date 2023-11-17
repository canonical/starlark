// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

// This file defines tests of the Value API.

import (
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
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

func TestDictSafeGetAllocs(t *testing.T) {
	const dictSize = 100
	expectedValue := starlark.String("expected")
	dict := starlark.NewDict(dictSize)
	for i := 0; i < dictSize; i++ {
		if err := dict.SetKey(starlark.MakeInt(i), expectedValue); err != nil {
			t.Fatal(err)
		}
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			if value, found, err := dict.SafeGet(thread, starlark.MakeInt(i)); err != nil {
				st.Error(err)
			} else if found != (i < dictSize) {
				st.Errorf("SafeGet returned found=%t but expected found=%t", found, i < dictSize)
			} else if found && value != starlark.String("expected") {
				st.Error("unexpected return value: expected %#v but got %#v", expectedValue, value)
			}
		}
	})
}
