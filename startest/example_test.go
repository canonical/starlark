package startest_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func Example() {
	// func TestFoo(t *testing.T) {
	TestFoo := func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.IOSafe)

		st.AddValue("foo", &Foo{bar: "bar"})
		st.AddLocal("my_local", 100)

		st.RunString(`
			assert.eq(type(foo), 'foo')
			assert.eq(str(foo), '<foo>')
			assert.true(foo)

			assert.eq(foo.bar, "bar")
			assert.eq(foo.baz(), 103)

			assert.fails(lambda: foo.baz('asdf'), "got 1 arguments?, want 0")

			foo.bar = "baz"
			assert.eq(foo.bar, "baz")

			foo.bar = "bar"
			assert.eq(foo.bar, "bar")
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

type Foo struct {
	bar    string
	frozen bool
}

var _ starlark.Value = &Foo{}
var _ starlark.HasAttrs = &Foo{}
var _ starlark.HasSetField = &Foo{}

// Implement starlark.Value
func (f *Foo) Type() string          { return "foo" }
func (f *Foo) String() string        { return "<foo>" }
func (f *Foo) Freeze()               { f.frozen = true }
func (f *Foo) Truth() starlark.Bool  { return starlark.Bool(f.bar != "") }
func (f *Foo) Hash() (uint32, error) { return starlark.String(f.bar).Hash() }

// Implement starlark.HasAttrs
func (f *Foo) AttrNames() []string { return []string{"bar", "baz"} }
func (f *Foo) Attr(name string) (starlark.Value, error) {
	switch name {
	case "bar":
		return starlark.String(f.bar), nil
	case "baz":
		return fooBaz.BindReceiver(f), nil
	}

	return nil, nil
}

// Implement starlark.HasSetField
func (f *Foo) SetField(name string, val starlark.Value) error {
	if f.frozen {
		return errors.New("Foo is frozen")
	}

	if name == "bar" {
		if s, ok := val.(starlark.String); ok {
			f.bar = string(s)
			return nil
		}
		return fmt.Errorf("foo.bar expected a string")
	}

	return errors.New("No such field .bar")
}

// Example implementation of a foo.baz starlark method
var fooBaz = starlark.NewBuiltinWithSafety("baz", starlark.IOSafe, func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
		return nil, err
	}

	local := thread.Local("my_local").(int)

	recv := b.Receiver().(*Foo)

	return starlark.MakeInt(local + len(recv.bar)), nil
})
