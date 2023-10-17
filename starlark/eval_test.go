// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/canonical/starlark/internal/chunkedfile"
	"github.com/canonical/starlark/lib/json"
	starlarkmath "github.com/canonical/starlark/lib/math"
	"github.com/canonical/starlark/lib/proto"
	"github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
	"github.com/canonical/starlark/starlarktest"
	"github.com/canonical/starlark/startest"
	"github.com/canonical/starlark/syntax"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "google.golang.org/protobuf/types/descriptorpb" // example descriptor needed for lib/proto tests
)

// A test may enable non-standard options by containing (e.g.) "option:recursion".
func getOptions(src string) *syntax.FileOptions {
	return &syntax.FileOptions{
		Set:               option(src, "set"),
		While:             option(src, "while"),
		TopLevelControl:   option(src, "toplevelcontrol"),
		GlobalReassign:    option(src, "globalreassign"),
		LoadBindsGlobally: option(src, "loadbindsglobally"),
		Recursion:         option(src, "recursion"),
	}
}

func option(chunk, name string) bool {
	return strings.Contains(chunk, "option:"+name)
}

func TestEvalExpr(t *testing.T) {
	// This is mostly redundant with the new *.star tests.
	// TODO(adonovan): move checks into *.star files and
	// reduce this to a mere unit test of starlark.Eval.
	thread := new(starlark.Thread)
	for _, test := range []struct{ src, want string }{
		{`123`, `123`},
		{`-1`, `-1`},
		{`"a"+"b"`, `"ab"`},
		{`1+2`, `3`},

		// lists
		{`[]`, `[]`},
		{`[1]`, `[1]`},
		{`[1,]`, `[1]`},
		{`[1, 2]`, `[1, 2]`},
		{`[2 * x for x in [1, 2, 3]]`, `[2, 4, 6]`},
		{`[2 * x for x in [1, 2, 3] if x > 1]`, `[4, 6]`},
		{`[(x, y) for x in [1, 2] for y in [3, 4]]`,
			`[(1, 3), (1, 4), (2, 3), (2, 4)]`},
		{`[(x, y) for x in [1, 2] if x == 2 for y in [3, 4]]`,
			`[(2, 3), (2, 4)]`},
		// tuples
		{`()`, `()`},
		{`(1)`, `1`},
		{`(1,)`, `(1,)`},
		{`(1, 2)`, `(1, 2)`},
		{`(1, 2, 3, 4, 5)`, `(1, 2, 3, 4, 5)`},
		{`1, 2`, `(1, 2)`},
		// dicts
		{`{}`, `{}`},
		{`{"a": 1}`, `{"a": 1}`},
		{`{"a": 1,}`, `{"a": 1}`},

		// conditional
		{`1 if 3 > 2 else 0`, `1`},
		{`1 if "foo" else 0`, `1`},
		{`1 if "" else 0`, `0`},

		// indexing
		{`["a", "b"][0]`, `"a"`},
		{`["a", "b"][1]`, `"b"`},
		{`("a", "b")[0]`, `"a"`},
		{`("a", "b")[1]`, `"b"`},
		{`"aΩb"[0]`, `"a"`},
		{`"aΩb"[1]`, `"\xce"`},
		{`"aΩb"[3]`, `"b"`},
		{`{"a": 1}["a"]`, `1`},
		{`{"a": 1}["b"]`, `key "b" not in dict`},
		{`{}[[]]`, `unhashable type: list`},
		{`{"a": 1}[[]]`, `unhashable type: list`},
		{`[x for x in range(3)]`, "[0, 1, 2]"},
	} {
		var got string
		if v, err := starlark.Eval(thread, "<expr>", test.src, nil); err != nil {
			got = err.Error()
		} else {
			got = v.String()
		}
		if got != test.want {
			t.Errorf("eval %s = %s, want %s", test.src, got, test.want)
		}
	}
}

func TestExecFile(t *testing.T) {
	testdata := starlarktest.DataFile("starlark", ".")
	thread := &starlark.Thread{Load: load}
	starlarktest.SetReporter(thread, t)
	proto.SetPool(thread, protoregistry.GlobalFiles)
	for _, file := range []string{
		"testdata/assign.star",
		"testdata/bool.star",
		"testdata/builtins.star",
		"testdata/bytes.star",
		"testdata/control.star",
		"testdata/dict.star",
		"testdata/float.star",
		"testdata/function.star",
		"testdata/int.star",
		"testdata/json.star",
		"testdata/list.star",
		"testdata/math.star",
		"testdata/misc.star",
		"testdata/proto.star",
		"testdata/set.star",
		"testdata/string.star",
		"testdata/time.star",
		"testdata/tuple.star",
		"testdata/recursion.star",
		"testdata/module.star",
		"testdata/while.star",
	} {
		filename := filepath.Join(testdata, file)
		for _, chunk := range chunkedfile.Read(filename, t) {
			predeclared := starlark.StringDict{
				"hasfields": starlark.NewBuiltin("hasfields", newHasFields),
				"fibonacci": fib{},
				"struct":    starlark.NewBuiltin("struct", starlarkstruct.Make),
			}

			opts := getOptions(chunk.Source)
			_, err := starlark.ExecFileOptions(opts, thread, filename, chunk.Source, predeclared)
			switch err := err.(type) {
			case *starlark.EvalError:
				found := false
				for i := range err.CallStack {
					posn := err.CallStack.At(i).Pos
					if posn.Filename() == filename {
						chunk.GotError(int(posn.Line), err.Error())
						found = true
						break
					}
				}
				if !found {
					t.Error(err.Backtrace())
				}
			case nil:
				// success
			default:
				t.Errorf("\n%s", err)
			}
			chunk.Done()
		}
	}
}

// A fib is an iterable value representing the infinite Fibonacci sequence.
type fib struct{}

func (t fib) Freeze()                    {}
func (t fib) String() string             { return "fib" }
func (t fib) Type() string               { return "fib" }
func (t fib) Truth() starlark.Bool       { return true }
func (t fib) Hash() (uint32, error)      { return 0, fmt.Errorf("fib is unhashable") }
func (t fib) Iterate() starlark.Iterator { return &fibIterator{0, 1} }

type fibIterator struct{ x, y int }

func (it *fibIterator) Next(p *starlark.Value) bool {
	*p = starlark.MakeInt(it.x)
	it.x, it.y = it.y, it.x+it.y
	return true
}
func (it *fibIterator) Done() {}

func (*fibIterator) Err() error { return nil }

// load implements the 'load' operation as used in the evaluator tests.
func load(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	if module == "assert.star" {
		return starlarktest.LoadAssertModule()
	}
	if module == "json.star" {
		return starlark.StringDict{"json": json.Module}, nil
	}
	if module == "time.star" {
		return starlark.StringDict{"time": time.Module}, nil
	}
	if module == "math.star" {
		return starlark.StringDict{"math": starlarkmath.Module}, nil
	}
	if module == "proto.star" {
		return starlark.StringDict{"proto": proto.Module}, nil
	}

	// TODO(adonovan): test load() using this execution path.
	filename := filepath.Join(filepath.Dir(thread.CallFrame(0).Pos.Filename()), module)
	return starlark.ExecFile(thread, filename, nil, nil)
}

func newHasFields(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args)+len(kwargs) > 0 {
		return nil, fmt.Errorf("%s: unexpected arguments", b.Name())
	}
	return &hasfields{attrs: make(map[string]starlark.Value)}, nil
}

// hasfields is a test-only implementation of HasAttrs.
// It permits any field to be set.
// Clients will likely want to provide their own implementation,
// so we don't have any public implementation.
type hasfields struct {
	attrs  starlark.StringDict
	frozen bool
}

var (
	_ starlark.HasAttrs  = (*hasfields)(nil)
	_ starlark.HasBinary = (*hasfields)(nil)
)

func (hf *hasfields) String() string        { return "hasfields" }
func (hf *hasfields) Type() string          { return "hasfields" }
func (hf *hasfields) Truth() starlark.Bool  { return true }
func (hf *hasfields) Hash() (uint32, error) { return 42, nil }

func (hf *hasfields) Freeze() {
	if !hf.frozen {
		hf.frozen = true
		for _, v := range hf.attrs {
			v.Freeze()
		}
	}
}

func (hf *hasfields) Attr(name string) (starlark.Value, error) { return hf.attrs[name], nil }

func (hf *hasfields) SetField(name string, val starlark.Value) error {
	if hf.frozen {
		return fmt.Errorf("cannot set field on a frozen hasfields")
	}
	if strings.HasPrefix(name, "no") { // for testing
		return starlark.NoSuchAttrError(fmt.Sprintf("no .%s field", name))
	}
	hf.attrs[name] = val
	return nil
}

func (hf *hasfields) AttrNames() []string {
	names := make([]string, 0, len(hf.attrs))
	for key := range hf.attrs {
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

func (hf *hasfields) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	// This method exists so we can exercise 'list += x'
	// where x is not Iterable but defines list+x.
	if op == syntax.PLUS {
		if _, ok := y.(*starlark.List); ok {
			return starlark.MakeInt(42), nil // list+hasfields is 42
		}
	}
	return nil, nil
}

func TestParameterPassing(t *testing.T) {
	const filename = "parameters.go"
	const src = `
def a():
	return
def b(a, b):
	return a, b
def c(a, b=42):
	return a, b
def d(*args):
	return args
def e(**kwargs):
	return kwargs
def f(a, b=42, *args, **kwargs):
	return a, b, args, kwargs
def g(a, b=42, *args, c=123, **kwargs):
	return a, b, args, c, kwargs
def h(a, b=42, *, c=123, **kwargs):
	return a, b, c, kwargs
def i(a, b=42, *, c, d=123, e, **kwargs):
	return a, b, c, d, e, kwargs
def j(a, b=42, *args, c, d=123, e, **kwargs):
	return a, b, args, c, d, e, kwargs
`

	thread := new(starlark.Thread)
	globals, err := starlark.ExecFile(thread, filename, src, nil)
	if err != nil {
		t.Fatal(err)
	}

	// All errors are dynamic; see resolver for static errors.
	for _, test := range []struct{ src, want string }{
		// a()
		{`a()`, `None`},
		{`a(1)`, `function a accepts no arguments (1 given)`},

		// b(a, b)
		{`b()`, `function b missing 2 arguments (a, b)`},
		{`b(1)`, `function b missing 1 argument (b)`},
		{`b(a=1)`, `function b missing 1 argument (b)`},
		{`b(b=1)`, `function b missing 1 argument (a)`},
		{`b(1, 2)`, `(1, 2)`},
		{`b`, `<function b>`}, // asserts that b's parameter b was treated as a local variable
		{`b(1, 2, 3)`, `function b accepts 2 positional arguments (3 given)`},
		{`b(1, b=2)`, `(1, 2)`},
		{`b(1, a=2)`, `function b got multiple values for parameter "a"`},
		{`b(1, x=2)`, `function b got an unexpected keyword argument "x"`},
		{`b(a=1, b=2)`, `(1, 2)`},
		{`b(b=1, a=2)`, `(2, 1)`},
		{`b(b=1, a=2, x=1)`, `function b got an unexpected keyword argument "x"`},
		{`b(x=1, b=1, a=2)`, `function b got an unexpected keyword argument "x"`},

		// c(a, b=42)
		{`c()`, `function c missing 1 argument (a)`},
		{`c(1)`, `(1, 42)`},
		{`c(1, 2)`, `(1, 2)`},
		{`c(1, 2, 3)`, `function c accepts at most 2 positional arguments (3 given)`},
		{`c(1, b=2)`, `(1, 2)`},
		{`c(1, a=2)`, `function c got multiple values for parameter "a"`},
		{`c(a=1, b=2)`, `(1, 2)`},
		{`c(b=1, a=2)`, `(2, 1)`},

		// d(*args)
		{`d()`, `()`},
		{`d(1)`, `(1,)`},
		{`d(1, 2)`, `(1, 2)`},
		{`d(1, 2, k=3)`, `function d got an unexpected keyword argument "k"`},
		{`d(args=[])`, `function d got an unexpected keyword argument "args"`},

		// e(**kwargs)
		{`e()`, `{}`},
		{`e(1)`, `function e accepts 0 positional arguments (1 given)`},
		{`e(k=1)`, `{"k": 1}`},
		{`e(kwargs={})`, `{"kwargs": {}}`},

		// f(a, b=42, *args, **kwargs)
		{`f()`, `function f missing 1 argument (a)`},
		{`f(0)`, `(0, 42, (), {})`},
		{`f(0)`, `(0, 42, (), {})`},
		{`f(0, 1)`, `(0, 1, (), {})`},
		{`f(0, 1, 2)`, `(0, 1, (2,), {})`},
		{`f(0, 1, 2, 3)`, `(0, 1, (2, 3), {})`},
		{`f(a=0)`, `(0, 42, (), {})`},
		{`f(0, b=1)`, `(0, 1, (), {})`},
		{`f(0, a=1)`, `function f got multiple values for parameter "a"`},
		{`f(0, b=1, c=2)`, `(0, 1, (), {"c": 2})`},

		// g(a, b=42, *args, c=123, **kwargs)
		{`g()`, `function g missing 1 argument (a)`},
		{`g(0)`, `(0, 42, (), 123, {})`},
		{`g(0, 1)`, `(0, 1, (), 123, {})`},
		{`g(0, 1, 2)`, `(0, 1, (2,), 123, {})`},
		{`g(0, 1, 2, 3)`, `(0, 1, (2, 3), 123, {})`},
		{`g(a=0)`, `(0, 42, (), 123, {})`},
		{`g(0, b=1)`, `(0, 1, (), 123, {})`},
		{`g(0, a=1)`, `function g got multiple values for parameter "a"`},
		{`g(0, b=1, c=2, d=3)`, `(0, 1, (), 2, {"d": 3})`},

		// h(a, b=42, *, c=123, **kwargs)
		{`h()`, `function h missing 1 argument (a)`},
		{`h(0)`, `(0, 42, 123, {})`},
		{`h(0, 1)`, `(0, 1, 123, {})`},
		{`h(0, 1, 2)`, `function h accepts at most 2 positional arguments (3 given)`},
		{`h(a=0)`, `(0, 42, 123, {})`},
		{`h(0, b=1)`, `(0, 1, 123, {})`},
		{`h(0, a=1)`, `function h got multiple values for parameter "a"`},
		{`h(0, b=1, c=2)`, `(0, 1, 2, {})`},
		{`h(0, b=1, d=2)`, `(0, 1, 123, {"d": 2})`},
		{`h(0, b=1, c=2, d=3)`, `(0, 1, 2, {"d": 3})`},

		// i(a, b=42, *, c, d=123, e, **kwargs)
		{`i()`, `function i missing 3 arguments (a, c, e)`},
		{`i(0)`, `function i missing 2 arguments (c, e)`},
		{`i(0, 1)`, `function i missing 2 arguments (c, e)`},
		{`i(0, 1, 2)`, `function i accepts at most 2 positional arguments (3 given)`},
		{`i(0, 1, e=2)`, `function i missing 1 argument (c)`},
		{`i(0, 1, 2, 3)`, `function i accepts at most 2 positional arguments (4 given)`},
		{`i(a=0)`, `function i missing 2 arguments (c, e)`},
		{`i(0, b=1)`, `function i missing 2 arguments (c, e)`},
		{`i(0, a=1)`, `function i got multiple values for parameter "a"`},
		{`i(0, b=1, c=2)`, `function i missing 1 argument (e)`},
		{`i(0, b=1, d=2)`, `function i missing 2 arguments (c, e)`},
		{`i(0, b=1, c=2, d=3)`, `function i missing 1 argument (e)`},
		{`i(0, b=1, c=2, d=3, e=4)`, `(0, 1, 2, 3, 4, {})`},
		{`i(0, 1, b=1, c=2, d=3, e=4)`, `function i got multiple values for parameter "b"`},

		// j(a, b=42, *args, c, d=123, e, **kwargs)
		{`j()`, `function j missing 3 arguments (a, c, e)`},
		{`j(0)`, `function j missing 2 arguments (c, e)`},
		{`j(0, 1)`, `function j missing 2 arguments (c, e)`},
		{`j(0, 1, 2)`, `function j missing 2 arguments (c, e)`},
		{`j(0, 1, e=2)`, `function j missing 1 argument (c)`},
		{`j(0, 1, 2, 3)`, `function j missing 2 arguments (c, e)`},
		{`j(a=0)`, `function j missing 2 arguments (c, e)`},
		{`j(0, b=1)`, `function j missing 2 arguments (c, e)`},
		{`j(0, a=1)`, `function j got multiple values for parameter "a"`},
		{`j(0, b=1, c=2)`, `function j missing 1 argument (e)`},
		{`j(0, b=1, d=2)`, `function j missing 2 arguments (c, e)`},
		{`j(0, b=1, c=2, d=3)`, `function j missing 1 argument (e)`},
		{`j(0, b=1, c=2, d=3, e=4)`, `(0, 1, (), 2, 3, 4, {})`},
		{`j(0, 1, b=1, c=2, d=3, e=4)`, `function j got multiple values for parameter "b"`},
		{`j(0, 1, 2, c=3, e=4)`, `(0, 1, (2,), 3, 123, 4, {})`},
	} {
		var got string
		if v, err := starlark.Eval(thread, "<expr>", test.src, globals); err != nil {
			got = err.Error()
		} else {
			got = v.String()
		}
		if got != test.want {
			t.Errorf("eval %s = %s, want %s", test.src, got, test.want)
		}
	}
}

// TestPrint ensures that the Starlark print function calls
// Thread.Print, if provided.
func TestPrint(t *testing.T) {
	const src = `
print("hello")
def f(): print("hello", "world", sep=", ")
f()
`
	buf := new(bytes.Buffer)
	print := func(thread *starlark.Thread, msg string) {
		caller := thread.CallFrame(1)
		fmt.Fprintf(buf, "%s: %s: %s\n", caller.Pos, caller.Name, msg)
	}
	thread := &starlark.Thread{Print: print}
	if _, err := starlark.ExecFile(thread, "foo.star", src, nil); err != nil {
		t.Fatal(err)
	}
	want := "foo.star:2:6: <toplevel>: hello\n" +
		"foo.star:3:15: f: hello, world\n"
	if got := buf.String(); got != want {
		t.Errorf("output was %s, want %s", got, want)
	}
}

func reportEvalError(tb testing.TB, err error) {
	if err, ok := err.(*starlark.EvalError); ok {
		tb.Fatal(err.Backtrace())
	}
	tb.Fatal(err)
}

// TestInt exercises the Int.Int64 and Int.Uint64 methods.
// If we can move their logic into math/big, delete this test.
func TestInt(t *testing.T) {
	one := starlark.MakeInt(1)

	for _, test := range []struct {
		i          starlark.Int
		wantInt64  string
		wantUint64 string
	}{
		{starlark.MakeInt64(math.MinInt64).Sub(one), "error", "error"},
		{starlark.MakeInt64(math.MinInt64), "-9223372036854775808", "error"},
		{starlark.MakeInt64(-1), "-1", "error"},
		{starlark.MakeInt64(0), "0", "0"},
		{starlark.MakeInt64(1), "1", "1"},
		{starlark.MakeInt64(math.MaxInt64), "9223372036854775807", "9223372036854775807"},
		{starlark.MakeUint64(math.MaxUint64), "error", "18446744073709551615"},
		{starlark.MakeUint64(math.MaxUint64).Add(one), "error", "error"},
	} {
		gotInt64, gotUint64 := "error", "error"
		if i, ok := test.i.Int64(); ok {
			gotInt64 = fmt.Sprint(i)
		}
		if u, ok := test.i.Uint64(); ok {
			gotUint64 = fmt.Sprint(u)
		}
		if gotInt64 != test.wantInt64 {
			t.Errorf("(%s).Int64() = %s, want %s", test.i, gotInt64, test.wantInt64)
		}
		if gotUint64 != test.wantUint64 {
			t.Errorf("(%s).Uint64() = %s, want %s", test.i, gotUint64, test.wantUint64)
		}
	}
}

func backtrace(t *testing.T, err error) string {
	switch err := err.(type) {
	case *starlark.EvalError:
		return err.Backtrace()
	case nil:
		t.Fatalf("ExecFile succeeded unexpectedly")
	default:
		t.Fatalf("ExecFile failed with %v, wanted *EvalError", err)
	}
	panic("unreachable")
}

func TestBacktrace(t *testing.T) {
	// This test ensures continuity of the stack of active Starlark
	// functions, including propagation through built-ins such as 'min'.
	const src = `
def f(x): return 1//x
def g(x): return f(x)
def h(): return min([1, 2, 0], key=g)
def i(): return h()
i()
`
	thread := new(starlark.Thread)
	_, err := starlark.ExecFile(thread, "crash.star", src, nil)
	const want = `Traceback (most recent call last):
  crash.star:6:2: in <toplevel>
  crash.star:5:18: in i
  crash.star:4:20: in h
  <builtin>: in min
  crash.star:3:19: in g
  crash.star:2:19: in f
Error: floored division by zero`
	if got := backtrace(t, err); got != want {
		t.Errorf("error was %s, want %s", got, want)
	}

	// Additionally, ensure that errors originating in
	// Starlark and/or Go each have an accurate frame.
	// The topmost frame, if built-in, is not shown,
	// but the name of the built-in function is shown
	// as "Error in fn: ...".
	//
	// This program fails in Starlark (f) if x==0,
	// or in Go (string.join) if x is non-zero.
	const src2 = `
def f(): ''.join([1//i])
f()
`
	for i, want := range []string{
		0: `Traceback (most recent call last):
  crash.star:3:2: in <toplevel>
  crash.star:2:20: in f
Error: floored division by zero`,
		1: `Traceback (most recent call last):
  crash.star:3:2: in <toplevel>
  crash.star:2:17: in f
Error in join: join: in list, want string, got int`,
	} {
		globals := starlark.StringDict{"i": starlark.MakeInt(i)}
		_, err := starlark.ExecFile(thread, "crash.star", src2, globals)
		if got := backtrace(t, err); got != want {
			t.Errorf("error was %s, want %s", got, want)
		}
	}
}

func TestLoadBacktrace(t *testing.T) {
	// This test ensures that load() does NOT preserve stack traces,
	// but that API callers can get them with Unwrap().
	// For discussion, see:
	// https://github.com/google/starlark-go/pull/244
	const src = `
load('crash.star', 'x')
`
	const loadedSrc = `
def f(x):
  return 1 // x

f(0)
`
	thread := new(starlark.Thread)
	thread.Load = func(t *starlark.Thread, module string) (starlark.StringDict, error) {
		return starlark.ExecFile(new(starlark.Thread), module, loadedSrc, nil)
	}
	_, err := starlark.ExecFile(thread, "root.star", src, nil)

	const want = `Traceback (most recent call last):
  root.star:2:1: in <toplevel>
Error: cannot load crash.star: floored division by zero`
	if got := backtrace(t, err); got != want {
		t.Errorf("error was %s, want %s", got, want)
	}

	unwrapEvalError := func(err error) *starlark.EvalError {
		var result *starlark.EvalError
		for {
			if evalErr, ok := err.(*starlark.EvalError); ok {
				result = evalErr
			}

			err = errors.Unwrap(err)
			if err == nil {
				break
			}
		}
		return result
	}

	unwrappedErr := unwrapEvalError(err)
	const wantUnwrapped = `Traceback (most recent call last):
  crash.star:5:2: in <toplevel>
  crash.star:3:12: in f
Error: floored division by zero`
	if got := backtrace(t, unwrappedErr); got != wantUnwrapped {
		t.Errorf("error was %s, want %s", got, wantUnwrapped)
	}

}

// TestRepeatedExec parses and resolves a file syntax tree once then
// executes it repeatedly with different values of its predeclared variables.
func TestRepeatedExec(t *testing.T) {
	predeclared := starlark.StringDict{"x": starlark.None}
	_, prog, err := starlark.SourceProgram("repeat.star", "y = 2 * x", predeclared.Has)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		x, want starlark.Value
	}{
		{x: starlark.MakeInt(42), want: starlark.MakeInt(84)},
		{x: starlark.String("mur"), want: starlark.String("murmur")},
		{x: starlark.Tuple{starlark.None}, want: starlark.Tuple{starlark.None, starlark.None}},
	} {
		predeclared["x"] = test.x // update the values in dictionary
		thread := new(starlark.Thread)
		if globals, err := prog.Init(thread, predeclared); err != nil {
			t.Errorf("x=%v: %v", test.x, err) // exec error
		} else if eq, err := starlark.Equal(globals["y"], test.want); err != nil {
			t.Errorf("x=%v: %v", test.x, err) // comparison error
		} else if !eq {
			t.Errorf("x=%v: got y=%v, want %v", test.x, globals["y"], test.want)
		}
	}
}

// TestEmptyFilePosition ensures that even Programs
// from empty files have a valid position.
func TestEmptyPosition(t *testing.T) {
	var predeclared starlark.StringDict
	for _, content := range []string{"", "empty = False"} {
		_, prog, err := starlark.SourceProgram("hello.star", content, predeclared.Has)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := prog.Filename(), "hello.star"; got != want {
			t.Errorf("Program.Filename() = %q, want %q", got, want)
		}
	}
}

// TestUnpackUserDefined tests that user-defined
// implementations of starlark.Value may be unpacked.
func TestUnpackUserDefined(t *testing.T) {
	// success
	want := new(hasfields)
	var x *hasfields
	if err := starlark.UnpackArgs("unpack", starlark.Tuple{want}, nil, "x", &x); err != nil {
		t.Errorf("UnpackArgs failed: %v", err)
	}
	if x != want {
		t.Errorf("for x, got %v, want %v", x, want)
	}

	// failure
	err := starlark.UnpackArgs("unpack", starlark.Tuple{starlark.MakeInt(42)}, nil, "x", &x)
	if want := "unpack: for parameter x: got int, want hasfields"; fmt.Sprint(err) != want {
		t.Errorf("unpack args error = %q, want %q", err, want)
	}
}

type optionalStringUnpacker struct {
	str   string
	isSet bool
}

func (o *optionalStringUnpacker) Unpack(v starlark.Value) error {
	s, ok := starlark.AsString(v)
	if !ok {
		return fmt.Errorf("got %s, want string", v.Type())
	}
	o.str = s
	o.isSet = ok
	return nil
}

func TestUnpackCustomUnpacker(t *testing.T) {
	a := optionalStringUnpacker{}
	wantA := optionalStringUnpacker{str: "a", isSet: true}
	b := optionalStringUnpacker{str: "b"}
	wantB := optionalStringUnpacker{str: "b"}

	// Success
	if err := starlark.UnpackArgs("unpack", starlark.Tuple{starlark.String("a")}, nil, "a?", &a, "b?", &b); err != nil {
		t.Errorf("UnpackArgs failed: %v", err)
	}
	if a != wantA {
		t.Errorf("for a, got %v, want %v", a, wantA)
	}
	if b != wantB {
		t.Errorf("for b, got %v, want %v", b, wantB)
	}

	// failure
	err := starlark.UnpackArgs("unpack", starlark.Tuple{starlark.MakeInt(42)}, nil, "a", &a)
	if want := "unpack: for parameter a: got int, want string"; fmt.Sprint(err) != want {
		t.Errorf("unpack args error = %q, want %q", err, want)
	}
}

func TestUnpackNoneCoalescing(t *testing.T) {
	a := optionalStringUnpacker{str: "a"}
	wantA := optionalStringUnpacker{str: "a", isSet: false}
	b := optionalStringUnpacker{str: "b"}
	wantB := optionalStringUnpacker{str: "b", isSet: false}

	// Success
	args := starlark.Tuple{starlark.None}
	kwargs := []starlark.Tuple{starlark.Tuple{starlark.String("b"), starlark.None}}
	if err := starlark.UnpackArgs("unpack", args, kwargs, "a??", &a, "b??", &a); err != nil {
		t.Errorf("UnpackArgs failed: %v", err)
	}
	if a != wantA {
		t.Errorf("for a, got %v, want %v", a, wantA)
	}
	if b != wantB {
		t.Errorf("for b, got %v, want %v", b, wantB)
	}

	// failure
	err := starlark.UnpackArgs("unpack", starlark.Tuple{starlark.MakeInt(42)}, nil, "a??", &a)
	if want := "unpack: for parameter a: got int, want string"; fmt.Sprint(err) != want {
		t.Errorf("unpack args error = %q, want %q", err, want)
	}

	err = starlark.UnpackArgs("unpack", nil, []starlark.Tuple{
		starlark.Tuple{starlark.String("a"), starlark.None},
		starlark.Tuple{starlark.String("a"), starlark.None},
	}, "a??", &a)
	if want := "unpack: got multiple values for keyword argument \"a\""; fmt.Sprint(err) != want {
		t.Errorf("unpack args error = %q, want %q", err, want)
	}
}

func TestUnpackRequiredAfterOptional(t *testing.T) {
	// Assert 'c' is implicitly optional
	var a, b, c string
	args := starlark.Tuple{starlark.String("a")}
	if err := starlark.UnpackArgs("unpack", args, nil, "a", &a, "b?", &b, "c", &c); err != nil {
		t.Errorf("UnpackArgs failed: %v", err)
	}
}

func TestAsInt(t *testing.T) {
	for _, test := range []struct {
		val  starlark.Value
		ptr  interface{}
		want string
	}{
		{starlark.MakeInt(42), new(int32), "42"},
		{starlark.MakeInt(-1), new(int32), "-1"},
		// Use Lsh not 1<<40 as the latter exceeds int if GOARCH=386.
		{starlark.MakeInt(1).Lsh(40), new(int32), "1099511627776 out of range (want value in signed 32-bit range)"},
		{starlark.MakeInt(-1).Lsh(40), new(int32), "-1099511627776 out of range (want value in signed 32-bit range)"},

		{starlark.MakeInt(42), new(uint16), "42"},
		{starlark.MakeInt(0xffff), new(uint16), "65535"},
		{starlark.MakeInt(0x10000), new(uint16), "65536 out of range (want value in unsigned 16-bit range)"},
		{starlark.MakeInt(-1), new(uint16), "-1 out of range (want value in unsigned 16-bit range)"},
	} {
		var got string
		if err := starlark.AsInt(test.val, test.ptr); err != nil {
			got = err.Error()
		} else {
			got = fmt.Sprint(reflect.ValueOf(test.ptr).Elem().Interface())
		}
		if got != test.want {
			t.Errorf("AsInt(%s, %T): got %q, want %q", test.val, test.ptr, got, test.want)
		}
	}
}

func TestDocstring(t *testing.T) {
	globals, _ := starlark.ExecFile(&starlark.Thread{}, "doc.star", `
def somefunc():
	"somefunc doc"
	return 0
`, nil)

	if globals["somefunc"].(*starlark.Function).Doc() != "somefunc doc" {
		t.Fatal("docstring not found")
	}
}

func TestFrameLocals(t *testing.T) {
	// trace prints a nice stack trace including argument
	// values of calls to Starlark functions.
	trace := func(thread *starlark.Thread) string {
		buf := new(bytes.Buffer)
		for i := 0; i < thread.CallStackDepth(); i++ {
			fr := thread.DebugFrame(i)
			fmt.Fprintf(buf, "%s(", fr.Callable().Name())
			if fn, ok := fr.Callable().(*starlark.Function); ok {
				for i := 0; i < fn.NumParams(); i++ {
					if i > 0 {
						buf.WriteString(", ")
					}
					name, _ := fn.Param(i)
					fmt.Fprintf(buf, "%s=%s", name, fr.Local(i))
				}
			} else {
				buf.WriteString("...") // a built-in function
			}
			buf.WriteString(")\n")
		}
		return buf.String()
	}

	var got string
	builtin := func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		got = trace(thread)
		return starlark.None, nil
	}
	predeclared := starlark.StringDict{
		"builtin": starlark.NewBuiltin("builtin", builtin),
	}
	_, err := starlark.ExecFile(&starlark.Thread{}, "foo.star", `
def f(x, y): builtin()
def g(z): f(z, z*z)
g(7)
`, predeclared)
	if err != nil {
		t.Errorf("ExecFile failed: %v", err)
	}

	var want = `
builtin(...)
f(x=7, y=49)
g(z=7)
<toplevel>()
`[1:]
	if got != want {
		t.Errorf("got <<%s>>, want <<%s>>", got, want)
	}
}

type badType string

func (b *badType) String() string        { return "badType" }
func (b *badType) Type() string          { return "badType:" + string(*b) } // panics if b==nil
func (b *badType) Truth() starlark.Bool  { return true }
func (b *badType) Hash() (uint32, error) { return 0, nil }
func (b *badType) Freeze()               {}

var _ starlark.Value = new(badType)

// TestUnpackErrorBadType verifies that the Unpack functions fail
// gracefully when a parameter's default value's Type method panics.
func TestUnpackErrorBadType(t *testing.T) {
	for _, test := range []struct {
		x    *badType
		want string
	}{
		{new(badType), "got NoneType, want badType"},       // Starlark type name
		{nil, "got NoneType, want *starlark_test.badType"}, // Go type name
	} {
		err := starlark.UnpackArgs("f", starlark.Tuple{starlark.None}, nil, "x", &test.x)
		if err == nil {
			t.Errorf("UnpackArgs succeeded unexpectedly")
			continue
		}
		if !strings.Contains(err.Error(), test.want) {
			t.Errorf("UnpackArgs error %q does not contain %q", err, test.want)
		}
	}
}

// Regression test for github.com/google/starlark-go/issues/233.
func TestREPLChunk(t *testing.T) {
	thread := new(starlark.Thread)
	globals := make(starlark.StringDict)
	exec := func(src string) {
		f, err := syntax.Parse("<repl>", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
			t.Fatal(err)
		}
	}

	exec("x = 0; y = 0")
	if got, want := fmt.Sprintf("%v %v", globals["x"], globals["y"]), "0 0"; got != want {
		t.Fatalf("chunk1: got %s, want %s", got, want)
	}

	exec("x += 1; y = y + 1")
	if got, want := fmt.Sprintf("%v %v", globals["x"], globals["y"]), "1 1"; got != want {
		t.Fatalf("chunk2: got %s, want %s", got, want)
	}
}

func TestCancel(t *testing.T) {
	// A thread cancelled before it begins executes no code.
	{
		thread := new(starlark.Thread)
		thread.Cancel("nope")
		_, err := starlark.ExecFile(thread, "precancel.star", `x = 1//0`, nil)
		if fmt.Sprint(err) != "Starlark computation cancelled: nope" {
			t.Errorf("execution returned error %q, want cancellation", err)
		}

		// cancellation is sticky
		_, err = starlark.ExecFile(thread, "precancel.star", `x = 1//0`, nil)
		if fmt.Sprint(err) != "Starlark computation cancelled: nope" {
			t.Errorf("execution returned error %q, want cancellation", err)
		}
	}
	// A thread cancelled during a built-in executes no more code.
	{
		thread := new(starlark.Thread)
		predeclared := starlark.StringDict{
			"stopit": starlark.NewBuiltin("stopit", func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				thread.Cancel(fmt.Sprint(args[0]))
				return starlark.None, nil
			}),
		}
		_, err := starlark.ExecFile(thread, "stopit.star", `msg = 'nope'; stopit(msg); x = 1//0`, predeclared)
		if fmt.Sprint(err) != `Starlark computation cancelled: "nope"` {
			t.Errorf("execution returned error %q, want cancellation", err)
		}
	}
}

func TestExecutionSteps(t *testing.T) {
	// A Thread records the number of computation steps.
	thread := new(starlark.Thread)
	countSteps := func(n int) (uint64, error) {
		predeclared := starlark.StringDict{"n": starlark.MakeInt(n)}
		steps0 := thread.ExecutionSteps()
		_, err := starlark.ExecFile(thread, "steps.star", `squares = [x*x for x in range(n)]`, predeclared)
		return thread.ExecutionSteps() - steps0, err
	}
	steps100, err := countSteps(1000)
	if err != nil {
		t.Errorf("execution failed: %v", err)
	}
	steps10000, err := countSteps(100000)
	if err != nil {
		t.Errorf("execution failed: %v", err)
	}
	if ratio := float64(steps10000) / float64(steps100); ratio < 99 || ratio > 101 {
		t.Errorf("computation steps did not increase linearly: f(100)=%d, f(10000)=%d, ratio=%g, want ~100", steps100, steps10000, ratio)
	}

	// Exceeding the step limit causes cancellation.
	thread.SetMaxExecutionSteps(1000)
	_, err = countSteps(1000)
	if fmt.Sprint(err) != "Starlark computation cancelled: too many steps" {
		t.Errorf("execution returned error %q, want cancellation", err)
	}

	thread.SetMaxExecutionSteps(thread.ExecutionSteps() + 100)
	thread.Uncancel()
	_, err = countSteps(1)
	if err != nil {
		t.Errorf("execution returned error %q, want nil", err)
	}
}

// TestDeps fails if the interpreter proper (not the REPL, etc) sprouts new external dependencies.
// We may expand the list of permitted dependencies, but should do so deliberately, not casually.
func TestDeps(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("'go list' failed: %s", err)
	}
	for _, pkg := range strings.Split(string(out), "\n") {
		// Does pkg have form "domain.name/dir"?
		slash := strings.IndexByte(pkg, '/')
		dot := strings.IndexByte(pkg, '.')
		if 0 < dot && dot < slash {
			if strings.HasPrefix(pkg, "github.com/canonical/starlark/") ||
				strings.HasPrefix(pkg, "golang.org/x/sys/") {
				continue // permitted dependencies
			}
			t.Errorf("new interpreter dependency: %s", pkg)
		}
	}
}

// TestPanicSafety ensures that a panic from an application-defined
// built-in may traverse the interpreter safely; see issue #411.
func TestPanicSafety(t *testing.T) {
	predeclared := starlark.StringDict{
		"panic": starlark.NewBuiltin("panic", func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			panic(args[0])
		}),
		"list": starlark.NewList([]starlark.Value{starlark.MakeInt(0)}),
	}

	// This program is executed twice, using the same Thread,
	// and panics both times, with values 1 and 2, while
	// main is on the stack and a for-loop is active.
	//
	// It mutates list, a predeclared variable.
	// This operation would fail if the previous
	// for-loop failed to close its iterator during the panic.
	//
	// It also calls main a second time without recursion enabled.
	// This operation would fail if the previous
	// call failed to pop main from the stack during the panic.
	const src = `
list[0] += 1

def main():
    for x in list:
        panic(x)

main()
`
	thread := new(starlark.Thread)
	for _, i := range []int{1, 2} {
		// Use a func to limit the scope of recover.
		func() {
			defer func() {
				if got := fmt.Sprint(recover()); got != fmt.Sprint(i) {
					t.Fatalf("recover: got %v, want %v", got, i)
				}
			}()
			v, err := starlark.ExecFile(thread, "panic.star", src, predeclared)
			if err != nil {
				t.Fatalf("ExecFile returned error %q, expected panic", err)
			} else {
				t.Fatalf("ExecFile returned %v, expected panic", v)
			}
		}()
	}
}

func TestCheckExecutionSteps(t *testing.T) {
	const maxSteps = 10000

	thread := new(starlark.Thread)
	thread.SetMaxExecutionSteps(maxSteps)

	if err := thread.CheckExecutionSteps(maxSteps / 2); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := thread.CheckExecutionSteps(2 * maxSteps); err == nil {
		t.Errorf("expected error")
	} else if err.Error() != "too many steps" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConcurrentCheckExecutionStepsUsage(t *testing.T) {
	const stepPeak = math.MaxInt64 ^ (math.MaxInt64 >> 1)
	const maxSteps = stepPeak + 1
	const repetitions = 1_000_000

	thread := &starlark.Thread{}
	thread.SetMaxExecutionSteps(maxSteps)
	thread.AddExecutionSteps(stepPeak - 1)

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		// Flip between 1000...00 and 0111...11 allocations
		for i := 0; i < repetitions; i++ {
			thread.AddExecutionSteps(1)
			thread.AddExecutionSteps(-1)
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < repetitions; i++ {
			// Check 1000...01 not exceeded
			if err := thread.CheckExecutionSteps(1); err != nil {
				t.Errorf("unexpected error: %v", err)
				break
			}
		}
		wg.Done()
	}()

	wg.Wait()
}

func TestAddExecutionStepsOk(t *testing.T) {
	const expectedDelta = 10000

	thread := new(starlark.Thread)
	thread.SetMaxExecutionSteps(2 * expectedDelta)

	if err := thread.AddExecutionSteps(expectedDelta); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if actualDelta := thread.ExecutionSteps(); actualDelta != expectedDelta {
		t.Errorf("incorrect number of steps added: expected %d but got %d", expectedDelta, actualDelta)
	}

	if _, err := starlark.ExecFile(thread, "add_execution_steps", "", nil); err != nil {
		t.Errorf("unexpected cancellation: %v", err)
	}
}

func TestAddExecutionStepsFail(t *testing.T) {
	const maxSteps = 10000
	const stepsToAdd = 2 * maxSteps

	thread := new(starlark.Thread)
	thread.SetMaxExecutionSteps(maxSteps)

	if err := thread.AddExecutionSteps(stepsToAdd); err == nil {
		t.Errorf("expected error")
	} else if err.Error() != "too many steps" {
		t.Errorf("unexpected error: %v", err)
	} else if steps := thread.ExecutionSteps(); steps != stepsToAdd {
		t.Errorf("incorrect number of steps recorded: expected %v but got %v", stepsToAdd, steps)
	}

	if _, err := starlark.ExecFile(thread, "add_execution_steps", "", nil); err == nil {
		t.Errorf("expected cancellation")
	} else if err.Error() != "Starlark computation cancelled: too many steps" {
		t.Errorf("unexpected error: %v", err)
	}

	const expectedStepsAfterExec = stepsToAdd + 1
	if err := thread.AddExecutionSteps(maxSteps / 2); err == nil {
		t.Errorf("expected error")
	} else if err.Error() != "too many steps" {
		t.Errorf("unexpected error: %v", err)
	} else if steps := thread.ExecutionSteps(); steps != expectedStepsAfterExec {
		t.Errorf("incorrect number of steps recorded: expected %v but got %v", expectedStepsAfterExec, steps)
	}
}

func TestConcurrentAddExecutionStepsUsage(t *testing.T) {
	const expectedSteps = 1_000_000

	thread := &starlark.Thread{}
	thread.SetMaxExecutionSteps(expectedSteps)

	wg := sync.WaitGroup{}
	wg.Add(2)

	callAddExecutionSteps := func(n uint) {
		for i := uint(0); i < n; i++ {
			if err := thread.AddExecutionSteps(1); err != nil {
				t.Errorf("unexpected error %v", err)
				break
			}
		}
		wg.Done()
	}

	go callAddExecutionSteps(expectedSteps / 2)
	go callAddExecutionSteps(expectedSteps / 2)

	wg.Wait()

	if steps := thread.ExecutionSteps(); steps != expectedSteps {
		t.Errorf("concurrent thread.AddExecutionSteps contains a race, expected %d steps recorded but got %d", expectedSteps, steps)
	}
}

func TestThreadPermits(t *testing.T) {
	const threadSafety = starlark.CPUSafe | starlark.MemSafe
	t.Run("Safety=Allowed", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(threadSafety)

		if !thread.Permits(starlark.Safe) {
			t.Errorf("allowed safety not permitted")
		}
	})

	t.Run("Safety=Forbidden", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(threadSafety)

		if thread.Permits(threadSafety &^ starlark.MemSafe) {
			t.Errorf("forbidden safety allowed")
		}
	})

	t.Run("Safety=Invalid", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(threadSafety)

		if thread.Permits(starlark.Safety(0xbad1091c) | threadSafety) {
			t.Errorf("invalid safety permitted")
		}
	})

	t.Run("ThreadSafety=Invalid", func(t *testing.T) {
		thread := &starlark.Thread{}
		const invalidSafety = starlark.Safety(0xa19ae)
		thread.RequireSafety(invalidSafety)

		if thread.Permits(starlark.Safe) {
			t.Errorf("invalid thread safety was permitted")
		}
	})
}

func TestThreadCheckPermits(t *testing.T) {
	const threadSafety = starlark.CPUSafe | starlark.MemSafe
	const prog = "func()"

	t.Run("Safety=Allowed", func(t *testing.T) {
		const allowedSafety = threadSafety | starlark.IOSafe

		thread := &starlark.Thread{}
		thread.RequireSafety(threadSafety)

		if err := thread.CheckPermits(allowedSafety); err != nil {
			t.Errorf("thread reported it did not permit acceptible safety: got unexpected error %v", err)
		}
	})

	t.Run("Safety=Forbidden", func(t *testing.T) {
		const forbiddenSafety = starlark.CPUSafe

		thread := &starlark.Thread{}
		thread.RequireSafety(threadSafety)

		if err := thread.CheckPermits(forbiddenSafety); err == nil {
			t.Errorf("thread failed to report that insufficient safety is unsafe")
		} else if err.Error() != "feature unavailable to the sandbox" {
			t.Errorf("unexpected error: %v", err)
		} else if safetyErr, ok := err.(*starlark.SafetyError); !ok {
			t.Errorf("expected starlark.SafetyError, got a %T: %v", err, err)
		} else if expectedMissing := threadSafety &^ forbiddenSafety; safetyErr.Missing != expectedMissing {
			t.Errorf("incorrect reported missing flags: expected %v but got %v", expectedMissing, safetyErr.Missing)
		}
	})

	t.Run("Safety=Invalid", func(t *testing.T) {
		thread := &starlark.Thread{}
		thread.RequireSafety(threadSafety)
		const invalidSafety = starlark.Safety(0xbad1091c)

		if err := thread.CheckPermits(invalidSafety | threadSafety); err == nil {
			t.Errorf("expected error checking invalid flags")
		} else if err.Error() != "internal error: invalid safety flags" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("ThreadSafety=Invalid", func(t *testing.T) {
		thread := &starlark.Thread{}
		const invalidSafety = starlark.Safety(0xa19ae)
		thread.RequireSafety(invalidSafety)

		if err := thread.CheckPermits(starlark.Safe); err == nil {
			t.Errorf("expected error checking against invalid flags")
		} else if err.Error() != "thread safety: internal error: invalid safety flags" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestThreadRequireSafetyDoesNotUnsetFlags(t *testing.T) {
	const initialSafety = starlark.CPUSafe | starlark.MemSafe
	const newSafety = starlark.IOSafe | starlark.TimeSafe
	const expectedSafety = initialSafety | newSafety

	thread := &starlark.Thread{}
	thread.RequireSafety(initialSafety)
	thread.RequireSafety(newSafety)

	if safety := starlark.ThreadSafety(thread); safety != expectedSafety {
		missing := safety &^ expectedSafety
		t.Errorf("missing safety flags %v, expected %v", missing.String(), expectedSafety.String())
	}
}

type safeBinaryAllocTest struct {
	name           string
	inputs         func(n int) (starlark.Value, syntax.Token, starlark.Value)
	assertNoAllocs bool
}

func (b safeBinaryAllocTest) Run(t *testing.T) {
	t.Run(b.name, func(t *testing.T) {
		if b.inputs == nil {
			t.Fatalf("binary test '%v' missing inputs field", b.name)
		}
		if b.name == "" {
			x, op, y := b.inputs(1)
			t.Fatalf("binary test of %v %v %v has empty name field", x.Type(), op, y.Type())
		}

		t.Run("nil-thread", func(t *testing.T) {
			defer func() {
				if err := recover(); err != nil {
					t.Errorf("unexpected panic: %v", err)
				}
			}()
			x, op, y := b.inputs(1)
			_, err := starlark.SafeBinary(nil, op, x, y)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})

		t.Run("small", func(t *testing.T) {
			st := startest.From(t)
			if b.assertNoAllocs {
				st.SetMaxAllocs(0)
			}
			st.RequireSafety(starlark.MemSafe)
			st.RunThread(func(thread *starlark.Thread) {
				x, op, y := b.inputs(1)
				for i := 0; i < st.N; i++ {
					result, err := starlark.SafeBinary(thread, op, x, y)
					if err != nil {
						st.Error(err)
					}
					st.KeepAlive(result)
				}
			})
		})

		t.Run("large", func(t *testing.T) {
			st := startest.From(t)
			if b.assertNoAllocs {
				st.SetMaxAllocs(0)
			}
			st.RequireSafety(starlark.MemSafe)
			st.RunThread(func(thread *starlark.Thread) {
				x, op, y := b.inputs(st.N)
				result, err := starlark.SafeBinary(thread, op, x, y)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			})
		})
	})
}

type unsafeTestValue struct{}

var _ starlark.Value = unsafeTestValue{}

func (uv unsafeTestValue) Freeze() {}
func (uv unsafeTestValue) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", uv.Type())
}
func (uv unsafeTestValue) String() string       { return "unsafeTestValue" }
func (uv unsafeTestValue) Truth() starlark.Bool { return starlark.False }
func (uv unsafeTestValue) Type() string         { return "unsafeTestValue" }

func TestSafeBinaryAllocs(t *testing.T) {
	t.Run("safety-respected", func(t *testing.T) {
		t.Run("unsafe-left", func(t *testing.T) {
			const expected = "feature unavailable to the sandbox"

			thread := &starlark.Thread{}
			thread.RequireSafety(starlark.Safe)
			_, err := starlark.SafeBinary(thread, syntax.PLUS, unsafeTestValue{}, starlark.True)
			if err == nil {
				t.Error("expected error")
			} else if err.Error() != expected {
				t.Errorf("unexpected error: %v", err)
			}
		})

		t.Run("unsafe-right", func(t *testing.T) {
			const expected = "feature unavailable to the sandbox"

			thread := &starlark.Thread{}
			thread.RequireSafety(starlark.Safe)
			_, err := starlark.SafeBinary(thread, syntax.PLUS, starlark.True, unsafeTestValue{})
			if err == nil {
				t.Error("expected error")
			} else if err.Error() != expected {
				t.Errorf("unexpected error: %v", err)
			}
		})
	})

	t.Run("+", func(t *testing.T) {
		t.Run("in-starlark", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.AddValue("t", starlark.Tuple{starlark.True})
			st.RunString(`
				for _ in st.ntimes():
					st.keep_alive(t + t)
			`)
		})

		tests := []safeBinaryAllocTest{{
			name: "string + string",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				str := starlark.String(strings.Repeat("x", n/2))
				return str, syntax.PLUS, str
			},
		}, {
			name: "int + int",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				shift := n - 1
				if shift < 0 {
					shift = 0
				}
				num := starlark.MakeInt(1).Lsh(uint(shift))
				return num, syntax.PLUS, num
			},
		}, {
			name: "int + float",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				l := starlark.MakeInt(1).Lsh(308)
				r := starlark.Float(n)
				return l, syntax.PLUS, r
			},
		}, {
			name: "float + int",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				l := starlark.Float(n)
				r := starlark.MakeInt(1).Lsh(308)
				return l, syntax.PLUS, r
			},
		}, {
			name: "float + float",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				num := starlark.Float(n)
				return num, syntax.PLUS, num
			},
		}, {
			name: "list + list",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				lElems := make([]starlark.Value, n/2)
				rElems := make([]starlark.Value, n/2)
				for i := 0; i < n/2; i++ {
					lElems[i] = starlark.String("a")
					rElems[i] = starlark.String("b")
				}
				l := starlark.NewList(lElems)
				r := starlark.NewList(rElems)
				return l, syntax.PLUS, r
			},
		}, {
			name: "tuple + tuple",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				l := make(starlark.Tuple, n/2)
				r := make(starlark.Tuple, n/2)
				for i := 0; i < n/2; i++ {
					l[i] = starlark.String("a")
					r[i] = starlark.String("b")
				}
				return l, syntax.PLUS, r
			},
		}}
		for _, test := range tests {
			test.Run(t)
		}

	})

	t.Run("-", func(t *testing.T) {})

	t.Run("*", func(t *testing.T) {
		tests := []safeBinaryAllocTest{{
			name: "int * int",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				num := starlark.MakeInt(int(math.Sqrt(float64(n))))
				return num, syntax.STAR, num
			},
		}, {
			name: "int * float",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				sqrtN := math.Sqrt(float64(n))
				l := starlark.MakeInt(int(sqrtN))
				r := starlark.Float(sqrtN)
				return l, syntax.STAR, r
			},
		}, {
			name: "float * int",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				sqrtN := math.Sqrt(float64(n))
				l := starlark.Float(sqrtN)
				r := starlark.MakeInt(int(sqrtN))
				return l, syntax.STAR, r
			},
		}, {
			name: "float * float",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				num := starlark.Float(math.Sqrt(float64(n)))
				return num, syntax.STAR, num
			},
		}, {
			name: "bytes * int",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				sqrtN := int(math.Sqrt(float64(n)))
				l := starlark.Bytes(strings.Repeat("deadbeef", sqrtN))
				r := starlark.MakeInt(sqrtN)
				return l, syntax.STAR, r
			},
		}, {
			name: "int * bytes",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				sqrtN := int(math.Sqrt(float64(n)))
				l := starlark.MakeInt(sqrtN)
				r := starlark.Bytes(strings.Repeat("deadbeef", sqrtN))
				return l, syntax.STAR, r
			},
		}, {
			name: "string * int",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				sqrtN := int(math.Sqrt(float64(n)))
				l := starlark.String(strings.Repeat("deadbeef", sqrtN))
				r := starlark.MakeInt(sqrtN)
				return l, syntax.STAR, r
			},
		}, {
			name: "int * string",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				sqrtN := int(math.Sqrt(float64(n)))
				l := starlark.MakeInt(sqrtN)
				r := starlark.String(strings.Repeat("deadbeef", sqrtN))
				return l, syntax.STAR, r
			},
		}, {
			name: "int * list",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				sqrtN := int(math.Sqrt(float64(n)))
				l := starlark.MakeInt(sqrtN)
				rElems := make([]starlark.Value, sqrtN)
				for i := 0; i < len(rElems); i++ {
					rElems[i] = starlark.String("a")
				}
				r := starlark.NewList(rElems)
				return l, syntax.STAR, r
			},
		}, {
			name: "list * int",
			inputs: func(n int) (starlark.Value, syntax.Token, starlark.Value) {
				sqrtN := int(math.Sqrt(float64(n)))
				lElems := make([]starlark.Value, sqrtN)
				for i := 0; i < len(lElems); i++ {
					lElems[i] = starlark.String("a")
				}
				l := starlark.NewList(lElems)
				r := starlark.MakeInt(sqrtN)
				return l, syntax.STAR, r
			},
		}, {
			name: "int * tuple",
			inputs: func(n int) (starlark.Value,syntax.Token, starlark.Value) {
				sqrtN := int(math.Sqrt(float64(n)))
				l := starlark.MakeInt(sqrtN)
				r := make(starlark.Tuple, sqrtN)
				for i := 0; i < len(r); i++ {
					r[i] = starlark.String("r")
				}
				return l, syntax.STAR, r
			},
		}, {
			name: "tuple * int",
			inputs: func(n int) (starlark.Value,syntax.Token, starlark.Value) {
				sqrtN := int(math.Sqrt(float64(n)))
				l := make(starlark.Tuple, sqrtN)
				for i := 0; i < len(l); i++ {
					l[i] = starlark.String("l")
				}
				r := starlark.MakeInt(sqrtN)
				return l, syntax.STAR, r
			},
		}}
		for _, test := range tests {
			test.Run(t)
		}
	})

	t.Run("/", func(t *testing.T) {})

	t.Run("//", func(t *testing.T) {})

	t.Run("%", func(t *testing.T) {})

	t.Run("in", func(t *testing.T) {})

	t.Run("not in", func(t *testing.T) {})

	t.Run("|", func(t *testing.T) {})

	t.Run("&", func(t *testing.T) {})

	t.Run("^", func(t *testing.T) {})

	t.Run("<<", func(t *testing.T) {})

	t.Run(">>", func(t *testing.T) {})
}

func TestThreadEnsureStack(t *testing.T) {
	t.Run("positive-size", func(t *testing.T) {
		dummy := &testing.T{}
		st := startest.From(dummy)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			thread.EnsureStack(st.N)
		})
		if !dummy.Failed() {
			t.Error("no new frames preallocated")
		}
	})

	t.Run("negative-size", func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.Error("expected panic")
			}
		}()
		thread := &starlark.Thread{}
		thread.EnsureStack(10)
		thread.EnsureStack(-1)
	})
}
