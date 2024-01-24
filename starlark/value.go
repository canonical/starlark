// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package starlark provides a Starlark interpreter.
//
// Starlark values are represented by the Value interface.
// The following built-in Value types are known to the evaluator:
//
//	NoneType        -- NoneType
//	Bool            -- bool
//	Bytes           -- bytes
//	Int             -- int
//	Float           -- float
//	String          -- string
//	*List           -- list
//	Tuple           -- tuple
//	*Dict           -- dict
//	*Set            -- set
//	*Function       -- function (implemented in Starlark)
//	*Builtin        -- builtin_function_or_method (function or method implemented in Go)
//
// Client applications may define new data types that satisfy at least
// the Value interface.  Such types may provide additional operations by
// implementing any of these optional interfaces:
//
//	Callable        -- value is callable like a function
//	Comparable      -- value defines its own comparison operations
//	Iterable        -- value is iterable using 'for' loops
//	Sequence        -- value is iterable sequence of known length
//	Indexable       -- value is sequence with efficient random access
//	Mapping         -- value maps from keys to values, like a dictionary
//	HasBinary       -- value defines binary operations such as * and +
//	HasAttrs        -- value has readable fields or methods x.f
//	HasSetField     -- value has settable fields x.f
//	HasSetIndex     -- value supports element update using x[i]=y
//	HasSetKey       -- value supports map update using x[k]=v
//	HasUnary        -- value defines unary operations such as + and -
//
// Client applications may also define domain-specific functions in Go
// and make them available to Starlark programs.  Use NewBuiltin to
// construct a built-in value that wraps a Go function.  The
// implementation of the Go function may use UnpackArgs to make sense of
// the positional and keyword arguments provided by the caller.
//
// Starlark's None value is not equal to Go's nil. Go's nil is not a legal
// Starlark value, but the compiler will not stop you from converting nil
// to Value. Be careful to avoid allowing Go nil values to leak into
// Starlark data structures.
//
// The Compare operation requires two arguments of the same
// type, but this constraint cannot be expressed in Go's type system.
// (This is the classic "binary method problem".)
// So, each Value type's CompareSameType method is a partial function
// that compares a value only against others of the same type.
// Use the package's standalone Compare (or Equal) function to compare
// an arbitrary pair of values.
//
// To parse and evaluate a Starlark source file, use ExecFile.  The Eval
// function evaluates a single expression.  All evaluator functions
// require a Thread parameter which defines the "thread-local storage"
// of a Starlark thread and may be used to plumb application state
// through Starlark code and into callbacks.  When evaluation fails it
// returns an EvalError from which the application may obtain a
// backtrace of active Starlark calls.
package starlark // import "github.com/canonical/starlark/starlark"

// This file defines the data types of Starlark and their basic operations.

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/canonical/starlark/internal/compile"
	"github.com/canonical/starlark/syntax"
)

// Value is a value in the Starlark interpreter.
type Value interface {
	// String returns the string representation of the value.
	// Starlark string values are quoted as if by Python's repr.
	String() string

	// Type returns a short string describing the value's type.
	Type() string

	// Freeze causes the value, and all values transitively
	// reachable from it through collections and closures, to be
	// marked as frozen.  All subsequent mutations to the data
	// structure through this API will fail dynamically, making the
	// data structure immutable and safe for publishing to other
	// Starlark interpreters running concurrently.
	Freeze()

	// Truth returns the truth value of an object.
	Truth() Bool

	// Hash returns a function of x such that Equals(x, y) => Hash(x) == Hash(y).
	// Hash may fail if the value's type is not hashable, or if the value
	// contains a non-hashable value. The hash is used only by dictionaries and
	// is not exposed to the Starlark program.
	Hash() (uint32, error)
}

// SizeAware allows an object to declare its own size
type SizeAware interface {
	EstimateSize() int64
}

type SafeStringer interface {
	SafeString(thread *Thread, sb StringBuilder) error
}

// A Comparable is a value that defines its own equivalence relation and
// perhaps ordered comparisons.
type Comparable interface {
	Value
	// CompareSameType compares one value to another of the same Type().
	// The comparison operation must be one of EQL, NEQ, LT, LE, GT, or GE.
	// CompareSameType returns an error if an ordered comparison was
	// requested for a type that does not support it.
	//
	// Implementations that recursively compare subcomponents of
	// the value should use the CompareDepth function, not Compare, to
	// avoid infinite recursion on cyclic structures.
	//
	// The depth parameter is used to bound comparisons of cyclic
	// data structures.  Implementations should decrement depth
	// before calling CompareDepth and should return an error if depth
	// < 1.
	//
	// Client code should not call this method.  Instead, use the
	// standalone Compare or Equals functions, which are defined for
	// all pairs of operands.
	CompareSameType(op syntax.Token, y Value, depth int) (bool, error)
}

// A TotallyOrdered is a type whose values form a total order:
// if x and y are of the same TotallyOrdered type, then x must be less than y,
// greater than y, or equal to y.
//
// It is simpler than Comparable and should be preferred in new code,
// but if a type implements both interfaces, Comparable takes precedence.
type TotallyOrdered interface {
	Value
	// Cmp compares two values x and y of the same totally ordered type.
	// It returns negative if x < y, positive if x > y, and zero if the values are equal.
	//
	// Implementations that recursively compare subcomponents of
	// the value should use the CompareDepth function, not Cmp, to
	// avoid infinite recursion on cyclic structures.
	//
	// The depth parameter is used to bound comparisons of cyclic
	// data structures.  Implementations should decrement depth
	// before calling CompareDepth and should return an error if depth
	// < 1.
	//
	// Client code should not call this method.  Instead, use the
	// standalone Compare or Equals functions, which are defined for
	// all pairs of operands.
	Cmp(y Value, depth int) (int, error)
}

var (
	_ TotallyOrdered = Int{}
	_ TotallyOrdered = Float(0)
	_ Comparable     = False
	_ Comparable     = String("")
	_ Comparable     = (*Dict)(nil)
	_ Comparable     = (*List)(nil)
	_ Comparable     = Tuple(nil)
	_ Comparable     = (*Set)(nil)
)

// A Callable value f may be the operand of a function call, f(x).
//
// Clients should use the Call function, never the CallInternal method.
type Callable interface {
	Value
	Name() string
	CallInternal(thread *Thread, args Tuple, kwargs []Tuple) (Value, error)
}

type callableWithPosition interface {
	Callable
	Position() syntax.Position
}

var (
	_ Callable             = (*Builtin)(nil)
	_ Callable             = (*Function)(nil)
	_ callableWithPosition = (*Function)(nil)
)

// An Iterable abstracts a sequence of values.
// An iterable value may be iterated over by a 'for' loop or used where
// any other Starlark iterable is allowed.  Unlike a Sequence, the length
// of an Iterable is not necessarily known in advance of iteration.
type Iterable interface {
	Value
	Iterate() Iterator // must be followed by call to Iterator.Done
}

// A Sequence is a sequence of values of known length.
type Sequence interface {
	Iterable
	Len() int
}

var (
	_ Sequence = (*Dict)(nil)
	_ Sequence = (*Set)(nil)
)

// An Indexable is a sequence of known length that supports efficient random access.
// It is not necessarily iterable.
type Indexable interface {
	Value
	Index(i int) Value // requires 0 <= i < Len()
	Len() int
}

type SafeIndexable interface {
	Indexable
	SafeIndex(thread *Thread, i int) (Value, error) // requires 0 <= i < Len()
}

// A Sliceable is a sequence that can be cut into pieces with the slice operator (x[i:j:step]).
//
// All native indexable objects are sliceable.
// This is a separate interface for backwards-compatibility.
type Sliceable interface {
	Indexable
	// For positive strides (step > 0), 0 <= start <= end <= n.
	// For negative strides (step < 0), -1 <= end <= start < n.
	// The caller must ensure that the start and end indices are valid
	// and that step is non-zero.
	Slice(start, end, step int) Value
}

// A HasSetIndex is an Indexable value whose elements may be assigned (x[i] = y).
//
// The implementation should not add Len to a negative index as the
// evaluator does this before the call.
type HasSetIndex interface {
	Indexable
	SetIndex(index int, v Value) error
}

// A HasSafeSetIndex is an Indexable value whose elements may be assigned (x[i] = y),
// respecting the safety of the thread.
type HasSafeSetIndex interface {
	SafeIndexable

	SafeSetIndex(thread *Thread, index int, v Value) error
}

var (
	_ HasSetIndex     = (*List)(nil)
	_ HasSafeSetIndex = (*List)(nil)
	_ Indexable       = Tuple(nil)
	_ Indexable       = String("")
	_ Sliceable       = Tuple(nil)
	_ Sliceable       = String("")
	_ Sliceable       = (*List)(nil)
)

// An Iterator provides a sequence of values to the caller.
//
// The caller must call Done when the iterator is no longer needed.
// Operations that modify a sequence will fail if it has active iterators.
//
// Example usage:
//
//	iter := iterable.Iterator()
//	defer iter.Done()
//	var x Value
//	for iter.Next(&x) {
//		...
//	}
type Iterator interface {
	// If the iterator is exhausted, Next returns false.
	// Otherwise it sets *p to the current element of the sequence,
	// advances the iterator, and returns true.
	Next(p *Value) bool
	Done()
	Err() error
}

// A SafeIterator is an Iterator which abides by safety constraints.
//
// When a thread is available and safety is required, `BindThread`
// will be called before iteration.
type SafeIterator interface {
	Iterator
	SafetyAware

	BindThread(thread *Thread)
}

// A Mapping is a mapping from keys to values, such as a dictionary.
//
// If a type satisfies both Mapping and Iterable, the iterator yields
// the keys of the mapping.
type Mapping interface {
	Value
	// Get returns the value corresponding to the specified key,
	// or !found if the mapping does not contain the key.
	//
	// Get also helps define the behavior of "v in mapping".
	// The 'in' operator reports the 'found' component, ignoring
	// non-safety errors.
	Get(Value) (v Value, found bool, err error)
}

type SafeMapping interface {
	Mapping
	// SafeGet returns the value corresponding to the specified key,
	// or !found if the mapping does not contain the key.
	//
	// SafeGet also helps define the behavior of "v in mapping".
	// The 'in' operator reports the 'found' component, ignoring
	// non-safety errors.
	SafeGet(thread *Thread, key Value) (v Value, found bool, err error)
}

// An IterableMapping is a mapping that supports key enumeration.
type IterableMapping interface {
	Mapping
	Iterate() Iterator // see Iterable interface
	Items() []Tuple    // a new slice containing all key/value pairs
}

type SequenceMapping interface {
	IterableMapping
	Len() int
}

var _ SequenceMapping = (*Dict)(nil)

// A HasSetKey supports map update using x[k]=v syntax, like a dictionary.
type HasSetKey interface {
	Mapping
	SetKey(k, v Value) error
}

// A HasSafeSetKey supports map update using x[k]=v syntax, like a dictionary,
// respecting the safety of the thread.
type HasSafeSetKey interface {
	Mapping

	SafeSetKey(thread *Thread, k, v Value) error
}

var _ HasSetKey = (*Dict)(nil)
var _ HasSafeSetKey = (*Dict)(nil)

// A HasBinary value may be used as either operand of these binary operators:
// +   -   *   /   //   %   in   not in   |   &   ^   <<   >>
//
// The Side argument indicates whether the receiver is the left or right operand.
//
// An implementation may decline to handle an operation by returning (nil, nil).
// For this reason, clients should always call the standalone Binary(op, x, y)
// function rather than calling the method directly.
type HasBinary interface {
	Value
	Binary(op syntax.Token, y Value, side Side) (Value, error)
}

type Side bool

const (
	Left  Side = false
	Right Side = true
)

// A HasUnary value may be used as the operand of these unary operators:
// +   -   ~
//
// An implementation may decline to handle an operation by returning (nil, nil).
// For this reason, clients should always call the standalone Unary(op, x)
// function rather than calling the method directly.
type HasUnary interface {
	Value
	Unary(op syntax.Token) (Value, error)
}

type HasSafeUnary interface {
	HasUnary
	SafeUnary(thread *Thread, op syntax.Token) (Value, error)
}

// A HasAttrs value has fields or methods that may be read by a dot expression (y = x.f).
// Attribute names may be listed using the built-in 'dir' function.
//
// For implementation convenience, a result of (nil, nil) from Attr is
// interpreted as a "no such field or method" error. Implementations are
// free to return a more precise error.
type HasAttrs interface {
	Value
	Attr(name string) (Value, error) // returns (nil, nil) if attribute not present
	AttrNames() []string             // callers must not modify the result.
}

// A HasSafeAttrs value has fields or methods that may be read by a dot expression (y = x.f),
// respecting the safety of the thread.
// Attribute names may be listed using the built-in 'dir' function.
//
// In contrast to HasAttrs, the SafeAttr method follows standard the Go convention
// and returns either a value or an error. If the attribute does not exist, it
// returns ErrNoSuchAttr.
type HasSafeAttrs interface {
	HasAttrs
	SafeAttr(thread *Thread, name string) (Value, error)
}

var (
	_ HasSafeAttrs = String("")
	_ HasSafeAttrs = Bytes("")
	_ HasSafeAttrs = new(List)
	_ HasSafeAttrs = new(Dict)
	_ HasSafeAttrs = new(Set)
)

// A HasSetField value has fields that may be written by a dot expression (x.f = y).
//
// An implementation of SetField may return a NoSuchAttrError,
// in which case the runtime may augment the error message to
// warn of possible misspelling.
type HasSetField interface {
	HasAttrs
	SetField(name string, val Value) error
}

type HasSafeSetField interface {
	HasSetField
	SafeSetField(thread *Thread, name string, val Value) error
}

// A NoSuchAttrError may be returned by an implementation of
// HasAttrs.Attr or HasSetField.SetField to indicate that no such field
// exists. In that case the runtime may augment the error message to
// warn of possible misspelling.
type NoSuchAttrError string

func (e NoSuchAttrError) Error() string { return string(e) }

// NoneType is the type of None.  Its only legal value is None.
// (We represent it as a number, not struct{}, so that None may be constant.)
type NoneType byte

const None = NoneType(0)

func (NoneType) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	_, err := sb.WriteString("None")
	return err
}

func (NoneType) String() string        { return "None" }
func (NoneType) Type() string          { return "NoneType" }
func (NoneType) Freeze()               {} // immutable
func (NoneType) Truth() Bool           { return False }
func (NoneType) Hash() (uint32, error) { return 0, nil }

// Bool is the type of a Starlark bool.
type Bool bool

const (
	False Bool = false
	True  Bool = true
)

func (b Bool) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	_, err := sb.WriteString(b.String())
	return err
}

func (b Bool) String() string {
	if b {
		return "True"
	} else {
		return "False"
	}
}
func (b Bool) Type() string          { return "bool" }
func (b Bool) Freeze()               {} // immutable
func (b Bool) Truth() Bool           { return b }
func (b Bool) Hash() (uint32, error) { return uint32(b2i(bool(b))), nil }
func (x Bool) CompareSameType(op syntax.Token, y_ Value, depth int) (bool, error) {
	y := y_.(Bool)
	return threeway(op, b2i(bool(x))-b2i(bool(y))), nil
}

// Float is the type of a Starlark float.
type Float float64

func (f Float) format(buf StringBuilder, conv byte) error {
	ff := float64(f)
	if !isFinite(ff) {
		if math.IsInf(ff, +1) {
			if _, err := buf.WriteString("+inf"); err != nil {
				return err
			}
		} else if math.IsInf(ff, -1) {
			if _, err := buf.WriteString("-inf"); err != nil {
				return err
			}
		} else {
			if _, err := buf.WriteString("nan"); err != nil {
				return err
			}
		}
		return nil
	}

	// %g is the default format used by str.
	// It uses the minimum precision to avoid ambiguity,
	// and always includes a '.' or an 'e' so that the value
	// is self-evidently a float, not an int.
	if conv == 'g' || conv == 'G' {
		s := strconv.FormatFloat(ff, conv, -1, 64)
		if _, err := buf.WriteString(s); err != nil {
			return err
		}
		// Ensure result always has a decimal point if no exponent.
		// "123" -> "123.0"
		if strings.IndexByte(s, conv-'g'+'e') < 0 && strings.IndexByte(s, '.') < 0 {
			if _, err := buf.WriteString(".0"); err != nil {
				return err
			}
		}
		return nil
	}

	// %[eEfF] use 6-digit precision
	if _, err := buf.WriteString(strconv.FormatFloat(ff, conv, 6, 64)); err != nil {
		return err
	}
	return nil
}

func (f Float) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return writeValue(thread, sb, f, nil)
}

func (f Float) String() string { return toString(f) }
func (f Float) Type() string   { return "float" }
func (f Float) Freeze()        {} // immutable
func (f Float) Truth() Bool    { return f != 0.0 }
func (f Float) Hash() (uint32, error) {
	// Equal float and int values must yield the same hash.
	// TODO(adonovan): opt: if f is non-integral, and thus not equal
	// to any Int, we can avoid the Int conversion and use a cheaper hash.
	if isFinite(float64(f)) {
		return finiteFloatToInt(f).Hash()
	}
	return 1618033, nil // NaN, +/-Inf
}

func floor(f Float) Float { return Float(math.Floor(float64(f))) }

// isFinite reports whether f represents a finite rational value.
// It is equivalent to !math.IsNan(f) && !math.IsInf(f, 0).
func isFinite(f float64) bool {
	return math.Abs(f) <= math.MaxFloat64
}

// Cmp implements comparison of two Float values.
// Required by the TotallyOrdered interface.
func (f Float) Cmp(v Value, depth int) (int, error) {
	g := v.(Float)
	return floatCmp(f, g), nil
}

// floatCmp performs a three-valued comparison on floats,
// which are totally ordered with NaN > +Inf.
func floatCmp(x, y Float) int {
	if x > y {
		return +1
	} else if x < y {
		return -1
	} else if x == y {
		return 0
	}

	// At least one operand is NaN.
	if x == x {
		return -1 // y is NaN
	} else if y == y {
		return +1 // x is NaN
	}
	return 0 // both NaN
}

func (f Float) rational() *big.Rat { return new(big.Rat).SetFloat64(float64(f)) }

// AsFloat returns the float64 value closest to x.
// The f result is undefined if x is not a float or Int.
// The result may be infinite if x is a very large Int.
func AsFloat(x Value) (f float64, ok bool) {
	switch x := x.(type) {
	case Float:
		return float64(x), true
	case Int:
		return float64(x.Float()), true
	}
	return 0, false
}

func (x Float) Mod(y Float) Float {
	z := Float(math.Mod(float64(x), float64(y)))
	if (x < 0) != (y < 0) && z != 0 {
		z += y
	}
	return z
}

// Unary implements the operations +float and -float.
func (f Float) Unary(op syntax.Token) (Value, error) {
	return f.SafeUnary(nil, op)
}

func (f Float) SafeUnary(thread *Thread, op syntax.Token) (Value, error) {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return nil, err
	}
	switch op {
	case syntax.MINUS:
		if thread != nil {
			if err := thread.AddAllocs(floatSize); err != nil {
				return nil, err
			}
		}
		return -f, nil
	case syntax.PLUS:
		if thread != nil {
			if err := thread.AddAllocs(floatSize); err != nil {
				return nil, err
			}
		}
		return +f, nil
	}
	return nil, nil
}

// String is the type of a Starlark text string.
//
// A String encapsulates an an immutable sequence of bytes,
// but strings are not directly iterable. Instead, iterate
// over the result of calling one of these four methods:
// codepoints, codepoint_ords, elems, elem_ords.
//
// Strings typically contain text; use Bytes for binary strings.
// The Starlark spec defines text strings as sequences of UTF-k
// codes that encode Unicode code points. In this Go implementation,
// k=8, whereas in a Java implementation, k=16. For portability,
// operations on strings should aim to avoid assumptions about
// the value of k.
//
// Warning: the contract of the Value interface's String method is that
// it returns the value printed in Starlark notation,
// so s.String() or fmt.Sprintf("%s", s) returns a quoted string.
// Use string(s) or s.GoString() or fmt.Sprintf("%#v", s) to obtain the raw contents
// of a Starlark string as a Go string.
type String string

func (s String) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return syntax.QuoteWriter(sb, string(s), false)
}

func (s String) String() string        { return syntax.Quote(string(s), false) }
func (s String) GoString() string      { return string(s) }
func (s String) Type() string          { return "string" }
func (s String) Freeze()               {} // immutable
func (s String) Truth() Bool           { return len(s) > 0 }
func (s String) Hash() (uint32, error) { return hashString(string(s)), nil }
func (s String) Len() int              { return len(s) } // bytes
func (s String) Index(i int) Value     { return s[i : i+1] }
func (s String) SafeIndex(thread *Thread, i int) (Value, error) {
	const safety = MemSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return nil, err
	}
	if thread != nil {
		if err := thread.AddAllocs(StringTypeOverhead); err != nil {
			return nil, err
		}
	}
	return s[i : i+1], nil
}

func (s String) Slice(start, end, step int) Value {
	if step == 1 {
		return s[start:end]
	}

	sign := signum(step)
	var str []byte
	for i := start; signum(end-i) == sign; i += step {
		str = append(str, s[i])
	}
	return String(str)
}

func (s String) Attr(name string) (Value, error) { return builtinAttr(s, name, stringMethods) }
func (s String) AttrNames() []string             { return builtinAttrNames(stringMethods) }

func (s String) SafeAttr(thread *Thread, name string) (Value, error) {
	attr, err := safeBuiltinAttr(thread, s, name, stringMethods)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		if err := thread.AddAllocs(StringTypeOverhead); err != nil {
			return nil, err
		}
	}
	return attr, nil
}

func (x String) CompareSameType(op syntax.Token, y_ Value, depth int) (bool, error) {
	y := y_.(String)
	return threeway(op, strings.Compare(string(x), string(y))), nil
}

func AsString(x Value) (string, bool) { v, ok := x.(String); return string(v), ok }

// A stringElems is an iterable whose iterator yields a sequence of
// elements (bytes), either numerically or as successive substrings.
// It is an indexable sequence.
type stringElems struct {
	s    String
	ords bool
}

var (
	_ Iterable  = (*stringElems)(nil)
	_ Indexable = (*stringElems)(nil)
)

func (si stringElems) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return writeValue(thread, sb, si, nil)
}

func (si stringElems) String() string        { return toString(si) }
func (si stringElems) Type() string          { return "string.elems" }
func (si stringElems) Freeze()               {} // immutable
func (si stringElems) Truth() Bool           { return True }
func (si stringElems) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: %s", si.Type()) }
func (si stringElems) Iterate() Iterator     { return &stringElemsIterator{si: si, i: 0} }
func (si stringElems) Len() int              { return len(si.s) }
func (si stringElems) Index(i int) Value {
	if si.ords {
		return MakeInt(int(si.s[i]))
	} else {
		// TODO(adonovan): opt: preallocate canonical 1-byte strings
		// to avoid interface allocation.
		return si.s[i : i+1]
	}
}
func (si stringElems) SafeIndex(thread *Thread, i int) (Value, error) {
	const safety = MemSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return nil, err
	}
	if si.ords {
		result := Value(MakeInt(int(si.s[i])))
		if thread != nil {
			if err := thread.AddAllocs(EstimateSize(result)); err != nil {
				return nil, err
			}
		}
		return result, nil
	} else {
		if thread != nil {
			if err := thread.AddAllocs(StringTypeOverhead); err != nil {
				return nil, err
			}
		}
		return si.s[i : i+1], nil
	}
}

type stringElemsIterator struct {
	si     stringElems
	i      int
	thread *Thread
	err    error
}

var _ SafeIterator = &stringElemsIterator{}

func (it *stringElemsIterator) BindThread(thread *Thread) {
	it.thread = thread
}

func (it *stringElemsIterator) Next(p *Value) bool {
	if it.err != nil {
		return false
	}
	if it.i == len(it.si.s) {
		return false
	}
	if v, err := it.si.SafeIndex(it.thread, it.i); err != nil {
		it.err = err
		return false
	} else {
		*p = v
		it.i++
		return true
	}
}

func (it *stringElemsIterator) Done()               {}
func (it *stringElemsIterator) Err() error          { return it.err }
func (it *stringElemsIterator) Safety() SafetyFlags { return MemSafe | CPUSafe }

// A stringCodepoints is an iterable whose iterator yields a sequence of
// Unicode code points, either numerically or as successive substrings.
// It is not indexable.
type stringCodepoints struct {
	s    String
	ords bool
}

var _ Iterable = (*stringCodepoints)(nil)

func (si stringCodepoints) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return writeValue(thread, sb, si, nil)
}

func (si stringCodepoints) String() string        { return toString(si) }
func (si stringCodepoints) Type() string          { return "string.codepoints" }
func (si stringCodepoints) Freeze()               {} // immutable
func (si stringCodepoints) Truth() Bool           { return True }
func (si stringCodepoints) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: %s", si.Type()) }
func (si stringCodepoints) Iterate() Iterator     { return &stringCodepointsIterator{si: si, i: 0} }

type stringCodepointsIterator struct {
	si     stringCodepoints
	i      int
	thread *Thread
	err    error
}

var _ SafeIterator = &stringCodepointsIterator{}

func (it *stringCodepointsIterator) BindThread(thread *Thread) {
	it.thread = thread
}

var runeSize = EstimateSize(ord)

func (it *stringCodepointsIterator) Next(p *Value) bool {
	if it.err != nil {
		return false
	}
	s := it.si.s[it.i:]
	if s == "" {
		return false
	}
	r, sz := utf8.DecodeRuneInString(string(s))
	if !it.si.ords {
		if err := it.thread.AddAllocs(StringTypeOverhead); err != nil {
			it.err = err
			return false
		}
		if r == utf8.RuneError {
			*p = String(r)
		} else {
			*p = s[:sz]
		}
	} else {
		if err := it.thread.AddAllocs(runeSize); err != nil {
			it.err = err
			return false
		}
		*p = MakeInt(int(r))
	}
	it.i += sz
	return true
}

func (*stringCodepointsIterator) Done() {}

func (it *stringCodepointsIterator) Err() error          { return it.err }
func (it *stringCodepointsIterator) Safety() SafetyFlags { return MemSafe | CPUSafe }

// A Function is a function defined by a Starlark def statement or lambda expression.
// The initialization behavior of a Starlark module is also represented by a Function.
type Function struct {
	funcode  *compile.Funcode
	module   *module
	defaults Tuple
	freevars Tuple
}

// A module is the dynamic counterpart to a Program.
// All functions in the same program share a module.
type module struct {
	program     *compile.Program
	predeclared StringDict
	globals     []Value
	constants   []Value
}

// makeGlobalDict returns a new, unfrozen StringDict containing all global
// variables so far defined in the module.
func (m *module) makeGlobalDict() StringDict {
	r := make(StringDict, len(m.program.Globals))
	for i, id := range m.program.Globals {
		if v := m.globals[i]; v != nil {
			r[id.Name] = v
		}
	}
	return r
}

func (fn *Function) Name() string          { return fn.funcode.Name } // "lambda" for anonymous functions
func (fn *Function) Doc() string           { return fn.funcode.Doc }
func (fn *Function) Hash() (uint32, error) { return hashString(fn.funcode.Name), nil }
func (fn *Function) Freeze()               { fn.defaults.Freeze(); fn.freevars.Freeze() }
func (fn *Function) Type() string          { return "function" }
func (fn *Function) Truth() Bool           { return true }
func (fn *Function) String() string        { return toString(fn) }

func (fn *Function) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return writeValue(thread, sb, fn, nil)
}

// Globals returns a new, unfrozen StringDict containing all global
// variables so far defined in the function's module.
func (fn *Function) Globals() StringDict { return fn.module.makeGlobalDict() }

func (fn *Function) Position() syntax.Position { return fn.funcode.Pos }
func (fn *Function) NumParams() int            { return fn.funcode.NumParams }
func (fn *Function) NumKwonlyParams() int      { return fn.funcode.NumKwonlyParams }

// Param returns the name and position of the ith parameter,
// where 0 <= i < NumParams().
// The *args and **kwargs parameters are at the end
// even if there were optional parameters after *args.
func (fn *Function) Param(i int) (string, syntax.Position) {
	if i >= fn.NumParams() {
		panic(i)
	}
	id := fn.funcode.Locals[i]
	return id.Name, id.Pos
}

// ParamDefault returns the default value of the specified parameter
// (0 <= i < NumParams()), or nil if the parameter is not optional.
func (fn *Function) ParamDefault(i int) Value {
	if i < 0 || i >= fn.NumParams() {
		panic(i)
	}

	// fn.defaults omits all required params up to the first optional param. It
	// also does not include *args or **kwargs at the end.
	firstOptIdx := fn.NumParams() - len(fn.defaults)
	if fn.HasVarargs() {
		firstOptIdx--
	}
	if fn.HasKwargs() {
		firstOptIdx--
	}
	if i < firstOptIdx || i >= firstOptIdx+len(fn.defaults) {
		return nil
	}

	dflt := fn.defaults[i-firstOptIdx]
	if _, ok := dflt.(mandatory); ok {
		return nil
	}
	return dflt
}

func (fn *Function) HasVarargs() bool { return fn.funcode.HasVarargs }
func (fn *Function) HasKwargs() bool  { return fn.funcode.HasKwargs }

const nativeSafe = safetyFlagsLimit - 1

func (fn *Function) Safety() SafetyFlags { return nativeSafe }

// A Builtin is a function implemented in Go.
type Builtin struct {
	name string
	fn   func(thread *Thread, fn *Builtin, args Tuple, kwargs []Tuple) (Value, error)
	recv Value // for bound methods (e.g. "".startswith)

	safety SafetyFlags
}

func (b *Builtin) Name() string { return b.name }
func (b *Builtin) Freeze() {
	if b.recv != nil {
		b.recv.Freeze()
	}
}
func (b *Builtin) Hash() (uint32, error) {
	h := hashString(b.name)
	if b.recv != nil {
		h ^= 5521
	}
	return h, nil
}

func (b *Builtin) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return writeValue(thread, sb, b, nil)
}

func (b *Builtin) String() string  { return toString(b) }
func (b *Builtin) Receiver() Value { return b.recv }
func (b *Builtin) Type() string    { return "builtin_function_or_method" }
func (b *Builtin) CallInternal(thread *Thread, args Tuple, kwargs []Tuple) (Value, error) {
	return b.fn(thread, b, args, kwargs)
}
func (b *Builtin) Truth() Bool { return true }

func (b *Builtin) Safety() SafetyFlags              { return b.safety }
func (b *Builtin) DeclareSafety(safety SafetyFlags) { b.safety = safety }

// NewBuiltin returns a new 'builtin_function_or_method' value with the specified name
// and implementation.  It compares unequal with all other values.
func NewBuiltin(name string, fn func(thread *Thread, fn *Builtin, args Tuple, kwargs []Tuple) (Value, error)) *Builtin {
	return &Builtin{name: name, fn: fn}
}

// NewBuiltinWithSafety is a convenience function which, like NewBuiltin,
// returns a new `builtin_function_or_method` with the specified name and
// implementation, which compares unequal with all other values. The safety of
// this new builtin is declared as the provided flags.
//
// This function is equivalent to calling NewBuiltin and DeclareSafety on its
// result.
func NewBuiltinWithSafety(name string, safety SafetyFlags, fn func(*Thread, *Builtin, Tuple, []Tuple) (Value, error)) *Builtin {
	return &Builtin{name: name, fn: fn, safety: safety}
}

// BindReceiver returns a new Builtin value representing a method
// closure, that is, a built-in function bound to a receiver value.
//
// In the example below, the value of f is the string.index
// built-in method bound to the receiver value "abc":
//
//	f = "abc".index; f("a"); f("b")
//
// In the common case, the receiver is bound only during the call,
// but this still results in the creation of a temporary method closure:
//
//	"abc".index("a")
func (b *Builtin) BindReceiver(recv Value) *Builtin {
	return &Builtin{name: b.name, fn: b.fn, recv: recv, safety: b.safety}
}

// A *Dict represents a Starlark dictionary.
// The zero value of Dict is a valid empty dictionary.
// If you know the exact final number of entries,
// it is more efficient to call NewDict.
type Dict struct {
	ht hashtable
}

// NewDict returns a set with initial space for
// at least size insertions before rehashing.
func NewDict(size int) *Dict {
	dict := new(Dict)
	dict.ht.init(nil, size)
	return dict
}

func SafeNewDict(thread *Thread, size int) (*Dict, error) {
	if thread != nil {
		if size > 0 {
			if err := thread.AddSteps(int64(size)); err != nil {
				return nil, err
			}
		}
		if err := thread.AddAllocs(EstimateSize(&Dict{})); err != nil {
			return nil, err
		}
	}
	dict := new(Dict)
	if err := dict.ht.init(thread, size); err != nil {
		return nil, err
	}
	return dict, nil
}

func (d *Dict) Clear() error                                    { return d.ht.clear(nil) }
func (d *Dict) Delete(k Value) (v Value, found bool, err error) { return d.ht.delete(nil, k) }
func (d *Dict) Get(k Value) (v Value, found bool, err error)    { return d.ht.lookup(nil, k) }
func (d *Dict) Items() []Tuple                                  { return d.ht.items() }
func (d *Dict) Keys() []Value                                   { return d.ht.keys() }
func (d *Dict) Values() []Value                                 { return d.ht.values() }
func (d *Dict) Len() int                                        { return int(d.ht.len) }
func (d *Dict) Iterate() Iterator                               { return d.ht.iterate() }
func (d *Dict) SetKey(k, v Value) error                         { return d.ht.insert(nil, k, v) }
func (d *Dict) Type() string                                    { return "dict" }
func (d *Dict) Freeze()                                         { d.ht.freeze() }
func (d *Dict) Truth() Bool                                     { return d.Len() > 0 }
func (d *Dict) Hash() (uint32, error)                           { return 0, fmt.Errorf("unhashable type: dict") }
func (d *Dict) String() string                                  { return toString(d) }

func (d *Dict) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return writeValue(thread, sb, d, nil)
}

func (d *Dict) SafeGet(thread *Thread, k Value) (v Value, found bool, err error) {
	return d.ht.lookup(thread, k)
}

func (d *Dict) SafeSetKey(thread *Thread, k, v Value) error {
	if err := CheckSafety(thread, MemSafe|CPUSafe); err != nil {
		return err
	}
	if err := d.ht.insert(thread, k, v); err != nil {
		return err
	}
	return nil
}

func (x *Dict) Union(y *Dict) *Dict {
	z, _ := x.safeUnion(nil, y)
	return z
}

func (x *Dict) safeUnion(thread *Thread, y *Dict) (*Dict, error) {
	zLenHint := x.Len()
	if yLen := y.Len(); yLen > zLenHint {
		zLenHint = yLen
	}
	z, err := SafeNewDict(thread, zLenHint)
	if err != nil {
		return nil, err
	}
	if err := z.ht.addAll(thread, &x.ht); err != nil {
		return nil, err
	}
	if err := z.ht.addAll(thread, &y.ht); err != nil {
		return nil, err
	}
	return z, nil
}

func (d *Dict) Attr(name string) (Value, error) { return builtinAttr(d, name, dictMethods) }
func (d *Dict) AttrNames() []string             { return builtinAttrNames(dictMethods) }

func (d *Dict) SafeAttr(thread *Thread, name string) (Value, error) {
	return safeBuiltinAttr(thread, d, name, dictMethods)
}

func (x *Dict) CompareSameType(op syntax.Token, y_ Value, depth int) (bool, error) {
	y := y_.(*Dict)
	switch op {
	case syntax.EQL:
		ok, err := dictsEqual(x, y, depth)
		return ok, err
	case syntax.NEQ:
		ok, err := dictsEqual(x, y, depth)
		return !ok, err
	default:
		return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
	}
}

func dictsEqual(x, y *Dict, depth int) (bool, error) {
	if x.Len() != y.Len() {
		return false, nil
	}
	for e := x.ht.head; e != nil; e = e.next {
		key, xval := e.key, e.value

		if yval, found, _ := y.Get(key); !found {
			return false, nil
		} else if eq, err := EqualDepth(xval, yval, depth-1); err != nil {
			return false, err
		} else if !eq {
			return false, nil
		}
	}
	return true, nil
}

// A *List represents a Starlark list value.
type List struct {
	elems     []Value
	frozen    bool
	itercount uint32 // number of active iterators (ignored if frozen)
}

// NewList returns a list containing the specified elements.
// Callers should not subsequently modify elems.
func NewList(elems []Value) *List { return &List{elems: elems} }

func (l *List) Freeze() {
	if !l.frozen {
		l.frozen = true
		for _, elem := range l.elems {
			elem.Freeze()
		}
	}
}

// checkMutable reports an error if the list should not be mutated.
// verb+" list" should describe the operation.
func (l *List) checkMutable(verb string) error {
	if l.frozen {
		return fmt.Errorf("cannot %s frozen list", verb)
	}
	if l.itercount > 0 {
		return fmt.Errorf("cannot %s list during iteration", verb)
	}
	return nil
}

func (l *List) String() string        { return toString(l) }
func (l *List) Type() string          { return "list" }
func (l *List) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: list") }
func (l *List) Truth() Bool           { return l.Len() > 0 }
func (l *List) Len() int              { return len(l.elems) }
func (l *List) Index(i int) Value     { return l.elems[i] }
func (l *List) SafeIndex(thread *Thread, i int) (Value, error) {
	const safety = MemSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return nil, err
	}
	return l.elems[i], nil
}

func (l *List) Slice(start, end, step int) Value {
	if step == 1 {
		elems := append([]Value{}, l.elems[start:end]...)
		return NewList(elems)
	}

	sign := signum(step)
	var list []Value
	for i := start; signum(end-i) == sign; i += step {
		list = append(list, l.elems[i])
	}
	return NewList(list)
}

func (l *List) Attr(name string) (Value, error) { return builtinAttr(l, name, listMethods) }
func (l *List) AttrNames() []string             { return builtinAttrNames(listMethods) }

func (l *List) SafeAttr(thread *Thread, name string) (Value, error) {
	return safeBuiltinAttr(thread, l, name, listMethods)
}

func (l *List) Iterate() Iterator {
	if !l.frozen {
		l.itercount++
	}
	return &listIterator{l: l}
}

func (x *List) CompareSameType(op syntax.Token, y_ Value, depth int) (bool, error) {
	y := y_.(*List)
	// It's tempting to check x == y as an optimization here,
	// but wrong because a list containing NaN is not equal to itself.
	return sliceCompare(op, x.elems, y.elems, depth)
}

func sliceCompare(op syntax.Token, x, y []Value, depth int) (bool, error) {
	// Fast path: check length.
	if len(x) != len(y) && (op == syntax.EQL || op == syntax.NEQ) {
		return op == syntax.NEQ, nil
	}

	// Find first element that is not equal in both lists.
	for i := 0; i < len(x) && i < len(y); i++ {
		if eq, err := EqualDepth(x[i], y[i], depth-1); err != nil {
			return false, err
		} else if !eq {
			switch op {
			case syntax.EQL:
				return false, nil
			case syntax.NEQ:
				return true, nil
			default:
				return CompareDepth(op, x[i], y[i], depth-1)
			}
		}
	}

	return threeway(op, len(x)-len(y)), nil
}

type listIterator struct {
	l *List
	i int
}

var _ SafeIterator = &listIterator{}

func (it *listIterator) Next(p *Value) bool {
	if it.i < it.l.Len() {
		*p = it.l.elems[it.i]
		it.i++
		return true
	}
	return false
}

func (it *listIterator) Done() {
	if !it.l.frozen {
		it.l.itercount--
	}
}

func (it *listIterator) Safety() SafetyFlags       { return MemSafe | CPUSafe }
func (it *listIterator) BindThread(thread *Thread) {}
func (it *listIterator) Err() error                { return nil }

func (l *List) SetIndex(i int, v Value) error {
	if err := l.checkMutable("assign to element of"); err != nil {
		return err
	}
	l.elems[i] = v
	return nil
}

func (l *List) SafeSetIndex(thread *Thread, i int, v Value) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return l.SetIndex(i, v)
}

func (l *List) Append(v Value) error {
	if err := l.checkMutable("append to"); err != nil {
		return err
	}
	l.elems = append(l.elems, v)
	return nil
}

func (l *List) Clear() error {
	if err := l.checkMutable("clear"); err != nil {
		return err
	}
	for i := range l.elems {
		l.elems[i] = nil // aid GC
	}
	l.elems = l.elems[:0]
	return nil
}

// A Tuple represents a Starlark tuple value.
type Tuple []Value

func (t Tuple) Len() int          { return len(t) }
func (t Tuple) Index(i int) Value { return t[i] }
func (t Tuple) SafeIndex(thread *Thread, i int) (Value, error) {
	const safety = MemSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return nil, err
	}
	return t[i], nil
}

func (t Tuple) Slice(start, end, step int) Value {
	if step == 1 {
		return t[start:end]
	}

	sign := signum(step)
	var tuple Tuple
	for i := start; signum(end-i) == sign; i += step {
		tuple = append(tuple, t[i])
	}
	return tuple
}

func (t Tuple) Iterate() Iterator { return &tupleIterator{elems: t} }
func (t Tuple) Freeze() {
	for _, elem := range t {
		elem.Freeze()
	}
}

func (t Tuple) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return writeValue(thread, sb, t, nil)
}

func (t Tuple) String() string { return toString(t) }
func (t Tuple) Type() string   { return "tuple" }
func (t Tuple) Truth() Bool    { return len(t) > 0 }

func (x Tuple) CompareSameType(op syntax.Token, y_ Value, depth int) (bool, error) {
	y := y_.(Tuple)
	return sliceCompare(op, x, y, depth)
}

func (t Tuple) Hash() (uint32, error) {
	// Use same algorithm as Python.
	var x, mult uint32 = 0x345678, 1000003
	for _, elem := range t {
		y, err := elem.Hash()
		if err != nil {
			return 0, err
		}
		x = x ^ y*mult
		mult += 82520 + uint32(len(t)+len(t))
	}
	return x, nil
}

type tupleIterator struct{ elems Tuple }

var _ SafeIterator = &tupleIterator{}

func (it *tupleIterator) Next(p *Value) bool {
	if len(it.elems) > 0 {
		*p = it.elems[0]
		it.elems = it.elems[1:]
		return true
	}
	return false
}

func (it *tupleIterator) Done() {}

func (it *tupleIterator) BindThread(thread *Thread) {}
func (it *tupleIterator) Err() error                { return nil }
func (it *tupleIterator) Safety() SafetyFlags       { return MemSafe | CPUSafe }

// A Set represents a Starlark set value.
// The zero value of Set is a valid empty set.
// If you know the exact final number of elements,
// it is more efficient to call NewSet.
type Set struct {
	ht hashtable // values are all None
}

// NewSet returns a dictionary with initial space for
// at least size insertions before rehashing.
func NewSet(size int) *Set {
	set := new(Set)
	set.ht.init(nil, size)
	return set
}

func (s *Set) Delete(k Value) (found bool, err error) { _, found, err = s.ht.delete(nil, k); return }
func (s *Set) Clear() error                           { return s.ht.clear(nil) }
func (s *Set) Has(k Value) (found bool, err error)    { _, found, err = s.ht.lookup(nil, k); return }
func (s *Set) Insert(k Value) error                   { return s.ht.insert(nil, k, None) }
func (s *Set) Len() int                               { return int(s.ht.len) }
func (s *Set) Iterate() Iterator                      { return s.ht.iterate() }
func (s *Set) Type() string                           { return "set" }
func (s *Set) Freeze()                                { s.ht.freeze() }
func (s *Set) Hash() (uint32, error)                  { return 0, fmt.Errorf("unhashable type: set") }
func (s *Set) Truth() Bool                            { return s.Len() > 0 }
func (s *Set) String() string                         { return toString(s) }

func (s *Set) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return writeValue(thread, sb, s, nil)
}

func (s *Set) safeHas(thread *Thread, k Value) (found bool, err error) {
	_, found, err = s.ht.lookup(thread, k)
	return found, err
}

func (s *Set) Attr(name string) (Value, error) { return builtinAttr(s, name, setMethods) }
func (s *Set) AttrNames() []string             { return builtinAttrNames(setMethods) }

func (s *Set) SafeAttr(thread *Thread, name string) (Value, error) {
	return safeBuiltinAttr(thread, s, name, setMethods)
}

func (x *Set) CompareSameType(op syntax.Token, y_ Value, depth int) (bool, error) {
	y := y_.(*Set)
	switch op {
	case syntax.EQL:
		ok, err := setsEqual(x, y, depth)
		return ok, err
	case syntax.NEQ:
		ok, err := setsEqual(x, y, depth)
		return !ok, err
	case syntax.GE: // superset
		if x.Len() < y.Len() {
			return false, nil
		}
		iter := y.Iterate()
		defer iter.Done()
		return x.IsSuperset(iter)
	case syntax.LE: // subset
		if x.Len() > y.Len() {
			return false, nil
		}
		iter := y.Iterate()
		defer iter.Done()
		return x.IsSubset(iter)
	case syntax.GT: // proper superset
		if x.Len() <= y.Len() {
			return false, nil
		}
		iter := y.Iterate()
		defer iter.Done()
		return x.IsSuperset(iter)
	case syntax.LT: // proper subset
		if x.Len() >= y.Len() {
			return false, nil
		}
		iter := y.Iterate()
		defer iter.Done()
		return x.IsSubset(iter)
	default:
		return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
	}
}

func setsEqual(x, y *Set, depth int) (bool, error) {
	if x.Len() != y.Len() {
		return false, nil
	}
	for e := x.ht.head; e != nil; e = e.next {
		if found, _ := y.Has(e.key); !found {
			return false, nil
		}
	}
	return true, nil
}

func setFromIterator(iter Iterator) (*Set, error) {
	var x Value
	set := new(Set)
	for iter.Next(&x) {
		err := set.Insert(x)
		if err != nil {
			return set, err
		}
	}
	return set, nil
}

func (s *Set) clone(thread *Thread) (*Set, error) {
	set := new(Set)
	if thread != nil {
		if err := thread.AddAllocs(EstimateSize(set)); err != nil {
			return nil, err
		}
	}
	set.ht.init(thread, int(s.ht.len))
	for e := s.ht.head; e != nil; e = e.next {
		if err := set.ht.insert(thread, e.key, None); err != nil {
			return nil, err
		}
	}
	return set, nil
}

func (s *Set) Union(iter Iterator) (Value, error) {
	return s.safeUnion(nil, iter)
}

func (s *Set) safeUnion(thread *Thread, iter Iterator) (Value, error) {
	set, err := s.clone(thread)
	if err != nil {
		return nil, err
	}
	var x Value
	for iter.Next(&x) {
		if err := set.ht.insert(thread, x, None); err != nil {
			return nil, err
		}
	}
	return set, nil
}

func (s *Set) Difference(other Iterator) (Value, error) {
	diff, _ := s.clone(nil) // can't fail
	var x Value
	for other.Next(&x) {
		if _, err := diff.Delete(x); err != nil {
			return nil, err
		}
	}
	return diff, nil
}

func (s *Set) safeDifference(thread *Thread, other Iterator) (Value, error) {
	if err := CheckSafety(thread, MemSafe|CPUSafe|IOSafe); err != nil {
		return nil, err
	}

	diff, err := s.clone(thread)
	if err != nil {
		return nil, err
	}
	var x Value
	for other.Next(&x) {
		if _, _, err := diff.ht.delete(thread, x); err != nil {
			return nil, err
		}
	}
	return diff, nil
}

func (s *Set) IsSuperset(other Iterator) (bool, error) {
	var x Value
	for other.Next(&x) {
		found, err := s.Has(x)
		if err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func (s *Set) IsSubset(other Iterator) (bool, error) {
	if count, err := s.ht.count(nil, other); err != nil {
		return false, err
	} else {
		return count == s.Len(), nil
	}
}

func (s *Set) Intersection(other Iterator) (Value, error) {
	return s.safeIntersection(nil, other)
}

func (s *Set) safeIntersection(thread *Thread, other Iterator) (*Set, error) {
	if err := CheckSafety(thread, MemSafe|CPUSafe|IOSafe); err != nil {
		return nil, err
	}

	intersect := new(Set)
	if thread != nil {
		if err := thread.AddAllocs(EstimateSize(intersect)); err != nil {
			return nil, err
		}
	}
	var x Value
	for other.Next(&x) {
		_, found, err := s.ht.lookup(thread, x)
		if err != nil {
			return nil, err
		}
		if found {
			err = intersect.ht.insert(thread, x, None)
			if err != nil {
				return nil, err
			}
		}
	}
	return intersect, nil
}

func (s *Set) SymmetricDifference(other Iterator) (Value, error) {
	return s.safeSymmetricDifference(nil, other)
}

func (s *Set) safeSymmetricDifference(thread *Thread, other Iterator) (Value, error) {
	if err := CheckSafety(thread, MemSafe|CPUSafe); err != nil {
		return nil, err
	}

	diff, err := s.clone(thread)
	if err != nil {
		return nil, err
	}
	var x Value
	for other.Next(&x) {
		_, found, err := diff.ht.delete(thread, x)
		if err != nil {
			return nil, err
		}
		if !found {
			if err := diff.ht.insert(thread, x, None); err != nil {
				return nil, err
			}
		}
	}
	return diff, nil
}

// toString returns the string form of value v.
// It may be more efficient than v.String() for larger values.
func toString(v Value) string {
	buf := new(strings.Builder)
	writeValue(nil, buf, v, nil)
	return buf.String()
}

func safeToString(thread *Thread, v Value) (string, error) {
	buf := NewSafeStringBuilder(thread)
	if err := writeValue(thread, buf, v, nil); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// writeValue writes x to out.
//
// path is used to detect cycles.
// It contains the list of *List and *Dict values we're currently printing.
// (These are the only potentially cyclic structures.)
// Callers should generally pass nil for path.
// It is safe to re-use the same path slice for multiple calls.
func writeValue(thread *Thread, out StringBuilder, x Value, path []Value) error {
	switch x := x.(type) {
	case nil:
		if _, err := out.WriteString("<nil>"); err != nil { // indicates a bug
			return err
		}

	// These four cases are duplicates of T.String(), for efficiency.
	case NoneType:
		if _, err := out.WriteString("None"); err != nil {
			return err
		}

	case Float:
		if err := x.format(out, 'g'); err != nil {
			return err
		}

	case Int:
		if iSmall, iBig := x.get(); iBig != nil {
			if _, err := fmt.Fprintf(out, "%d", iBig); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(out, "%d", iSmall); err != nil {
				return err
			}
		}

	case Bool:
		if x {
			if _, err := out.WriteString("True"); err != nil {
				return err
			}
		} else {
			if _, err := out.WriteString("False"); err != nil {
				return err
			}
		}

	case String:
		if err := syntax.QuoteWriter(out, string(x), false); err != nil {
			return err
		}

	case stringElems:
		if err := syntax.QuoteWriter(out, string(x.s), false); err != nil {
			return err
		}

		var method string
		if x.ords {
			method = ".elem_ords()"
		} else {
			method = ".elems()"
		}

		if _, err := out.WriteString(method); err != nil {
			return err
		}

	case stringCodepoints:
		if err := syntax.QuoteWriter(out, string(x.s), false); err != nil {
			return err
		}

		var method string
		if x.ords {
			method = ".codepoint_ords()"
		} else {
			method = ".codepoints()"
		}

		if _, err := out.WriteString(method); err != nil {
			return err
		}

	case Bytes:
		if err := syntax.QuoteWriter(out, string(x), true); err != nil {
			return err
		}

	case *List:
		if err := out.WriteByte('['); err != nil {
			return err
		}
		if pathContains(path, x) {
			if _, err := out.WriteString("..."); err != nil { // list contains itself
				return err
			}
		} else {
			if thread != nil {
				// Add 1 step per element to match the cost of using SafeIterate.
				if err := thread.AddSteps(int64(len(x.elems))); err != nil {
					return err
				}
			}
			for i, elem := range x.elems {
				if i > 0 {
					if _, err := out.WriteString(", "); err != nil {
						return err
					}
				}
				if err := writeValue(thread, out, elem, append(path, x)); err != nil {
					return err
				}
			}
		}
		if err := out.WriteByte(']'); err != nil {
			return err
		}

	case Tuple:
		if err := out.WriteByte('('); err != nil {
			return err
		}
		if thread != nil {
			// Add 1 step per element to match the cost of using SafeIterate.
			if err := thread.AddSteps(int64(len(x))); err != nil {
				return err
			}
		}
		for i, elem := range x {
			if i > 0 {
				if _, err := out.WriteString(", "); err != nil {
					return err
				}
			}
			if err := writeValue(thread, out, elem, path); err != nil {
				return err
			}
		}
		if len(x) == 1 {
			if err := out.WriteByte(','); err != nil {
				return err
			}
		}
		if err := out.WriteByte(')'); err != nil {
			return err
		}

	case *Function:
		if _, err := fmt.Fprintf(out, "<function %s>", x.Name()); err != nil {
			return err
		}

	case *Builtin:
		if x.recv != nil {
			if _, err := fmt.Fprintf(out, "<built-in method %s of %s value>", x.Name(), x.recv.Type()); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(out, "<built-in function %s>", x.Name()); err != nil {
				return err
			}
		}

	case *Dict:
		if err := out.WriteByte('{'); err != nil {
			return err
		}
		if pathContains(path, x) {
			if _, err := out.WriteString("..."); err != nil { // dict contains itself
				return err
			}
		} else {
			sep := ""
			if thread != nil {
				// Add 1 step per element to match the cost of using SafeIterate.
				if err := thread.AddSteps(int64(x.ht.len)); err != nil {
					return err
				}
			}
			for e := x.ht.head; e != nil; e = e.next {
				k, v := e.key, e.value
				if _, err := out.WriteString(sep); err != nil {
					return err
				}
				if err := writeValue(thread, out, k, path); err != nil {
					return err
				}
				if _, err := out.WriteString(": "); err != nil {
					return err
				}
				if err := writeValue(thread, out, v, append(path, x)); err != nil { // cycle check
					return err
				}
				sep = ", "
			}
		}
		if err := out.WriteByte('}'); err != nil {
			return err
		}

	case *Set:
		if _, err := out.WriteString("set(["); err != nil {
			return err
		}
		if thread != nil {
			// Add 1 step per element to match the cost of using SafeIterate.
			if err := thread.AddSteps(int64(x.ht.len)); err != nil {
				return err
			}
		}
		for e := x.ht.head; e != nil; e = e.next {
			if e != x.ht.head {
				if _, err := out.WriteString(", "); err != nil {
					return err
				}
			}
			if err := writeValue(thread, out, e.key, path); err != nil {
				return err
			}
		}
		if _, err := out.WriteString("])"); err != nil {
			return err
		}

	case SafeStringer:
		if err := x.SafeString(thread, out); err != nil {
			return err
		}

	default:
		if err := CheckSafety(thread, NotSafe); err != nil {
			return err
		}
		if _, err := out.WriteString(x.String()); err != nil {
			return err
		}
	}

	return nil
}

func pathContains(path []Value, x Value) bool {
	for _, y := range path {
		if x == y {
			return true
		}
	}
	return false
}

// CompareLimit is the depth limit on recursive comparison operations such as == and <.
// Comparison of data structures deeper than this limit may fail.
var CompareLimit = 10

// Equal reports whether two Starlark values are equal.
func Equal(x, y Value) (bool, error) {
	if x, ok := x.(String); ok {
		return x == y, nil // fast path for an important special case
	}
	return EqualDepth(x, y, CompareLimit)
}

// EqualDepth reports whether two Starlark values are equal.
//
// Recursive comparisons by implementations of Value.CompareSameType
// should use EqualDepth to prevent infinite recursion.
func EqualDepth(x, y Value, depth int) (bool, error) {
	return CompareDepth(syntax.EQL, x, y, depth)
}

// Compare compares two Starlark values.
// The comparison operation must be one of EQL, NEQ, LT, LE, GT, or GE.
// Compare returns an error if an ordered comparison was
// requested for a type that does not support it.
//
// Recursive comparisons by implementations of Value.CompareSameType
// should use CompareDepth to prevent infinite recursion.
func Compare(op syntax.Token, x, y Value) (bool, error) {
	return CompareDepth(op, x, y, CompareLimit)
}

// CompareDepth compares two Starlark values.
// The comparison operation must be one of EQL, NEQ, LT, LE, GT, or GE.
// CompareDepth returns an error if an ordered comparison was
// requested for a pair of values that do not support it.
//
// The depth parameter limits the maximum depth of recursion
// in cyclic data structures.
func CompareDepth(op syntax.Token, x, y Value, depth int) (bool, error) {
	if depth < 1 {
		return false, fmt.Errorf("comparison exceeded maximum recursion depth")
	}
	if sameType(x, y) {
		if xcomp, ok := x.(Comparable); ok {
			return xcomp.CompareSameType(op, y, depth)
		}

		if xcomp, ok := x.(TotallyOrdered); ok {
			t, err := xcomp.Cmp(y, depth)
			if err != nil {
				return false, err
			}
			return threeway(op, t), nil
		}

		// use identity comparison
		switch op {
		case syntax.EQL:
			return x == y, nil
		case syntax.NEQ:
			return x != y, nil
		}
		return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
	}

	// different types

	// int/float ordered comparisons
	switch x := x.(type) {
	case Int:
		if y, ok := y.(Float); ok {
			var cmp int
			if y != y {
				cmp = -1 // y is NaN
			} else if !math.IsInf(float64(y), 0) {
				cmp = x.rational().Cmp(y.rational()) // y is finite
			} else if y > 0 {
				cmp = -1 // y is +Inf
			} else {
				cmp = +1 // y is -Inf
			}
			return threeway(op, cmp), nil
		}
	case Float:
		if y, ok := y.(Int); ok {
			var cmp int
			if x != x {
				cmp = +1 // x is NaN
			} else if !math.IsInf(float64(x), 0) {
				cmp = x.rational().Cmp(y.rational()) // x is finite
			} else if x > 0 {
				cmp = +1 // x is +Inf
			} else {
				cmp = -1 // x is -Inf
			}
			return threeway(op, cmp), nil
		}
	}

	// All other values of different types compare unequal.
	switch op {
	case syntax.EQL:
		return false, nil
	case syntax.NEQ:
		return true, nil
	}
	return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
}

func sameType(x, y Value) bool {
	return reflect.TypeOf(x) == reflect.TypeOf(y) || x.Type() == y.Type()
}

// threeway interprets a three-way comparison value cmp (-1, 0, +1)
// as a boolean comparison (e.g. x < y).
func threeway(op syntax.Token, cmp int) bool {
	switch op {
	case syntax.EQL:
		return cmp == 0
	case syntax.NEQ:
		return cmp != 0
	case syntax.LE:
		return cmp <= 0
	case syntax.LT:
		return cmp < 0
	case syntax.GE:
		return cmp >= 0
	case syntax.GT:
		return cmp > 0
	}
	panic(op)
}

func b2i(b bool) int {
	if b {
		return 1
	} else {
		return 0
	}
}

// Len returns the length of a string or sequence value,
// and -1 for all others.
//
// Warning: Len(x) >= 0 does not imply Iterate(x) != nil.
// A string has a known length but is not directly iterable.
func Len(x Value) int {
	switch x := x.(type) {
	case String:
		return x.Len()
	case Indexable:
		return x.Len()
	case Sequence:
		return x.Len()
	}
	return -1
}

// Iterate return a new iterator for the value if iterable, nil otherwise.
// If the result is non-nil, the caller must call Done when finished with it.
//
// Warning: Iterate(x) != nil does not imply Len(x) >= 0.
// Some iterables may have unknown length.
func Iterate(x Value) Iterator {
	if x, ok := x.(Iterable); ok {
		return x.Iterate()
	}
	return nil
}

// guardedIterator provides a wrapper around an iterator which performs
// optional actions on Next calls.
type guardedIterator struct {
	iter   SafeIterator
	thread *Thread
	err    error
}

var _ SafeIterator = &guardedIterator{}

func (gi *guardedIterator) Next(p *Value) bool {
	if gi.Err() != nil {
		return false
	}

	ok := gi.iter.Next(p)
	if ok {
		if err := gi.thread.AddSteps(1); err != nil {
			gi.err = err
			return false
		}
	}
	return ok
}
func (gi *guardedIterator) Done() { gi.iter.Done() }
func (gi *guardedIterator) Err() error {
	if gi.err != nil {
		return gi.err
	}
	return gi.iter.Err()
}

func (gi *guardedIterator) Safety() SafetyFlags {
	const wrapperSafety = MemSafe | CPUSafe
	return wrapperSafety & gi.iter.Safety()
}
func (gi *guardedIterator) BindThread(thread *Thread) { gi.thread = thread }

// SafeIterate creates an iterator which is bound then to the given
// thread. This iterator will check safety and respect sandboxing
// bounds as required. As a convenience for functions that may have
// a thread or not depending on external logic, if thread is nil
// the iterator is still returned without its safety being checked.
func SafeIterate(thread *Thread, x Value) (Iterator, error) {
	if x, ok := x.(Iterable); ok {
		iter := x.Iterate()

		if thread != nil {
			if safeIter, ok := iter.(SafeIterator); ok {
				safeIter.BindThread(thread)
				if err := thread.CheckPermits(safeIter); err != nil {
					return nil, err
				}
				if !thread.Permits(NotSafe) {
					safeIter = &guardedIterator{iter: safeIter}
					safeIter.BindThread(thread)
				}
				return safeIter, nil
			}
			if err := thread.CheckPermits(NotSafe); err != nil {
				return nil, err
			}
		}

		return iter, nil
	}

	return nil, ErrUnsupported
}

// Bytes is the type of a Starlark binary string.
//
// A Bytes encapsulates an immutable sequence of bytes.
// It is comparable, indexable, and sliceable, but not directly iterable;
// use bytes.elems() for an iterable view.
//
// In this Go implementation, the elements of 'string' and 'bytes' are
// both bytes, but in other implementations, notably Java, the elements
// of a 'string' are UTF-16 codes (Java chars). The spec abstracts text
// strings as sequences of UTF-k codes that encode Unicode code points,
// and operations that convert from text to binary incur UTF-k-to-UTF-8
// transcoding; conversely, conversion from binary to text incurs
// UTF-8-to-UTF-k transcoding. Because k=8 for Go, these operations
// are the identity function, at least for valid encodings of text.
type Bytes string

var (
	_ Comparable = Bytes("")
	_ Sliceable  = Bytes("")
	_ Indexable  = Bytes("")
)

func (b Bytes) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = MemSafe | IOSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	return syntax.QuoteWriter(sb, string(b), true)
}

func (b Bytes) String() string        { return syntax.Quote(string(b), true) }
func (b Bytes) Type() string          { return "bytes" }
func (b Bytes) Freeze()               {} // immutable
func (b Bytes) Truth() Bool           { return len(b) > 0 }
func (b Bytes) Hash() (uint32, error) { return String(b).Hash() }
func (b Bytes) Len() int              { return len(b) }
func (b Bytes) Index(i int) Value     { return b[i : i+1] }
func (b Bytes) SafeIndex(thread *Thread, i int) (Value, error) {
	const safety = MemSafe | CPUSafe
	if err := CheckSafety(thread, safety); err != nil {
		return nil, err
	}
	if thread != nil {
		if err := thread.AddAllocs(StringTypeOverhead); err != nil {
			return nil, err
		}
	}
	return b[i : i+1], nil
}

func (b Bytes) Attr(name string) (Value, error) { return builtinAttr(b, name, bytesMethods) }
func (b Bytes) AttrNames() []string             { return builtinAttrNames(bytesMethods) }

func (b Bytes) SafeAttr(thread *Thread, name string) (Value, error) {
	attr, err := safeBuiltinAttr(thread, b, name, bytesMethods)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		if err := thread.AddAllocs(StringTypeOverhead); err != nil {
			return nil, err
		}
	}
	return attr, nil
}

func (b Bytes) Slice(start, end, step int) Value {
	if step == 1 {
		return b[start:end]
	}

	sign := signum(step)
	var str []byte
	for i := start; signum(end-i) == sign; i += step {
		str = append(str, b[i])
	}
	return Bytes(str)
}

func (x Bytes) CompareSameType(op syntax.Token, y_ Value, depth int) (bool, error) {
	y := y_.(Bytes)
	return threeway(op, strings.Compare(string(x), string(y))), nil
}
