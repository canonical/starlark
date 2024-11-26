package starlarkstruct

import (
	"fmt"

	"github.com/canonical/starlark/starlark"
)

// A Module is a named collection of values,
// typically a suite of functions imported by a load statement.
//
// It differs from Struct primarily in that its string representation
// does not enumerate its fields.
type Module struct {
	Name    string
	Members starlark.StringDict
}

var _ starlark.HasSafeAttrs = (*Module)(nil)

func (m *Module) Attr(name string) (starlark.Value, error) { return m.Members[name], nil }
func (m *Module) AttrNames() []string                      { return m.Members.Keys() }
func (m *Module) Freeze()                                  { m.Members.Freeze() }
func (m *Module) Hash() (uint32, error)                    { return 0, fmt.Errorf("unhashable: %s", m.Type()) }
func (m *Module) String() string                           { return fmt.Sprintf("<module %q>", m.Name) }
func (m *Module) Truth() starlark.Bool                     { return true }
func (m *Module) Type() string                             { return "module" }

func (m *Module) SafeString(thread *starlark.Thread, sb starlark.StringBuilder) error {
	const safety = starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe | starlark.IOSafe
	if err := starlark.CheckSafety(thread, safety); err != nil {
		return err
	}
	_, err := fmt.Fprintf(sb, "<module %q>", m.Name)
	return err
}

func (m *Module) SafeAttr(thread *starlark.Thread, name string) (starlark.Value, error) {
	const safety = starlark.CPUSafe | starlark.MemSafe | starlark.TimeSafe | starlark.IOSafe
	if err := starlark.CheckSafety(thread, safety); err != nil {
		return nil, err
	}
	member, ok := m.Members[name]
	if !ok {
		return nil, starlark.ErrNoAttr
	}
	return member, nil
}

const MakeModuleSafety = starlark.CPUSafe | starlark.MemSafe | starlark.IOSafe

// MakeModule may be used as the implementation of a Starlark built-in
// function, module(name, **kwargs). It returns a new module with the
// specified name and members.
//
// An application can add 'module' to the Starlark environment like so:
//
//	globals := starlark.StringDict{
//		"module":  starlark.NewBuiltinWithSafety("module", starlarkstruct.MakeModuleSafety, starlarkstruct.MakeModule),
//	}
func MakeModule(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, nil, 1, &name); err != nil {
		return nil, err
	}
	if err := thread.AddSteps(int64(len(kwargs))); err != nil {
		return nil, err
	}
	resultSize := starlark.OldSafeAdd64(
		starlark.EstimateMakeSize(starlark.StringDict{}, len(kwargs)),
		starlark.EstimateSize(&Module{}),
	)
	if err := thread.AddAllocs(resultSize); err != nil {
		return nil, err
	}
	members := make(starlark.StringDict, len(kwargs))
	for _, kwarg := range kwargs {
		k := string(kwarg[0].(starlark.String))
		members[k] = kwarg[1]
	}
	return &Module{name, members}, nil
}
