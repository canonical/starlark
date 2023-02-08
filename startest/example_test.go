package startest_test

import (
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func Example() {
	// func TestFoo(t *testing.T) {
	TestFoo := func(t *testing.T) {
		st := startest.From(t)

		st.AddValue("foo", starlark.String("bar"))

		st.RunString(`
		assert.eq(type(st.N), "int")
		assert.eq(type(st.keep_alive), "builtin_function_or_method")
	`)
	}
	// }

	// Ignore this.
	t := &testing.T{}
	TestFoo(t)
	if !t.Failed() {
		fmt.Println("ok")
	}
	// Output:
	// ok
}

func ExampleST_RunString() {
	// func TestFoo(t *testing.T) {
	TestFoo := func(t *testing.T) {
		st := startest.From(t)

		st.AddValue("foo", starlark.String("bar"))

		st.RunString(`
		assert.eq(foo, "bar")
	`)
	}
	// }

	// Ignore this.
	t := &testing.T{}
	TestFoo(t)
	if !t.Failed() {
		fmt.Println("ok")
	}
	// Output:
	// ok
}

func ExampleST_RunThread() {
	// func TestFoo(t *testing.T) {
	TestFoo := func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.NotSafe)

		// Allow at most 4 bytes allocated per st.N.
		st.SetMaxAllocs(4)

		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				st.KeepAlive(new(int32))
			}
		})
	}
	// }

	// Ignore this.
	t := &testing.T{}
	TestFoo(t)
	if !t.Failed() {
		fmt.Println("ok")
	}
	// Output: ok
}

func ExampleST_SetMaxAllocs() {
	// func TestFoo(t *testing.T) {
	TestFoo := func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)

		// Declare max allowed allocations per st.N
		st.SetMaxAllocs(100)

		st.RunString(`
		for i in range(st.N):
			st.keep_alive(i)
	`)
	}
	// }

	// Ignore this.
	t := &testing.T{}
	TestFoo(t)
	if !t.Failed() {
		fmt.Println("ok")
	}
	// Output: ok
}

func ExampleST_AddLocal() {
	var local interface{}

	// func TestFoo(t *testing.T) {
	TestFoo := func(t *testing.T) {

		st := startest.From(t)
		st.RequireSafety(starlark.NotSafe)

		st.AddLocal("my_local", "foo")
		st.AddBuiltin(starlark.NewBuiltin("builtin", func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			local = thread.Local("my_local")
			return starlark.None, nil
		}))

		st.RunString(`
		builtin()
	`)
	}
	// }

	// Ignore this.
	t := &testing.T{}
	TestFoo(t)
	if !t.Failed() && local == "foo" {
		fmt.Println("ok")
	}
	// Output: ok
}
