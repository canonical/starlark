// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/canonical/starlark/internal/compile"
	"github.com/canonical/starlark/internal/spell"
	"github.com/canonical/starlark/resolve"
	"github.com/canonical/starlark/syntax"
)

// A Thread contains the state of a Starlark thread,
// such as its call stack and thread-local storage.
// The Thread is threaded throughout the evaluator.
type Thread struct {
	// Name is an optional name that describes the thread, for debugging.
	Name string

	// stack is the stack of (internal) call frames.
	stack []*frame

	// Print is the client-supplied implementation of the Starlark
	// 'print' function. If nil, fmt.Fprintln(os.Stderr, msg) is
	// used instead.
	Print func(thread *Thread, msg string)

	// Load is the client-supplied implementation of module loading.
	// Repeated calls with the same module name must return the same
	// module environment or error.
	// The error message need not include the module name.
	//
	// See example_test.go for some example implementations of Load.
	Load func(thread *Thread, module string) (StringDict, error)

	// steps counts abstract computation steps executed by this thread.
	steps, maxSteps uint64
	stepsLock       sync.Mutex

	// allocs counts the abstract memory units claimed by this resource pool
	allocs, maxAllocs uint64
	allocsLock        sync.Mutex

	// cancelReason records the reason from the first call to Cancel.
	cancelReason *string

	// locals holds arbitrary "thread-local" Go values belonging to the client.
	// They are accessible to the client but not to any Starlark program.
	locals map[string]interface{}

	// proftime holds the accumulated execution time since the last profile event.
	proftime time.Duration

	// requiredSafety holds the set of safety conditions which must be
	// satisfied by any builtin which is called when running this thread.
	requiredSafety Safety
}

// ExecutionSteps returns a count of abstract computation steps executed
// by this thread. It is incremented by the interpreter. It may be used
// as a measure of the approximate cost of Starlark execution, by
// computing the difference in its value before and after a computation.
//
// The precise meaning of "step" is not specified and may change.
func (thread *Thread) ExecutionSteps() uint64 {
	thread.stepsLock.Lock()
	defer thread.stepsLock.Unlock()

	return thread.steps
}

// SetMaxExecutionSteps sets a limit on the number of Starlark
// computation steps that may be executed by this thread. If the
// thread's step counter exceeds this limit, the interpreter calls
// thread.Cancel("too many steps").
func (thread *Thread) SetMaxExecutionSteps(max uint64) {
	thread.maxSteps = max
}

// CheckExecutionSteps returns an error if an increase in execution steps taken
// by this thread would be rejected by AddExecutionSteps.
//
// It is safe to call CheckExecutionSteps from any goroutine, even if the thread
// is actively executing.
func (thread *Thread) CheckExecutionSteps(delta uint64) error {
	thread.stepsLock.Lock()
	defer thread.stepsLock.Unlock()

	_, err := thread.simulateExecutionSteps(delta)
	return err
}

// AddExecutionSteps reports an increase in the number of execution steps taken
// by this thread. If the new total steps exceeds the limit defined by
// SetMaxExecutionSteps, the thread is cancelled and an error is returned.
//
// It is safe to call AddExecutionSteps from any goroutine, even if the thread
// is actively executing.
func (thread *Thread) AddExecutionSteps(delta uint64) error {
	thread.stepsLock.Lock()
	defer thread.stepsLock.Unlock()

	nextSteps, err := thread.simulateExecutionSteps(delta)
	thread.steps = nextSteps
	if err != nil {
		thread.Cancel(err.Error())
	}

	return err
}

// simulateExecutionSteps simulates a call to AddExecutionSteps returning the
// new total step-count and any error this would entail. No change is
// recorded.
func (thread *Thread) simulateExecutionSteps(delta uint64) (uint64, error) {
	if cancelReason := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&thread.cancelReason))); cancelReason != nil {
		return thread.steps, errors.New(*(*string)(cancelReason))
	}

	var nextExecutionSteps uint64
	if delta <= math.MaxUint64-thread.steps {
		nextExecutionSteps = thread.steps + delta
	} else {
		nextExecutionSteps = math.MaxUint64
	}

	if nextExecutionSteps > thread.maxSteps {
		return nextExecutionSteps, errors.New("too many steps")
	}

	return nextExecutionSteps, nil
}

// Allocs returns the total allocations reported to this thread via AddAllocs.
func (thread *Thread) Allocs() uint64 {
	return thread.allocs
}

// SetMaxAllocs sets the maximum allocations that may be reported to this thread
// via AddAllocs before Cancel is internally called. If max is zero or MaxUint64,
// the thread will not be cancelled.
func (thread *Thread) SetMaxAllocs(max uint64) {
	thread.maxAllocs = max
}

// RequireSafety makes the thread only accept functions that declare at least
// the provided safety.
func (thread *Thread) RequireSafety(safety Safety) {
	thread.requiredSafety |= safety
}

// Permits checks whether this thread would allow execution of the provided
// safety-aware value.
func (thread *Thread) Permits(value SafetyAware) bool {
	safety := value.Safety()
	return safety.CheckValid() == nil && safety.Contains(thread.requiredSafety)
}

// CheckPermits returns an error if this thread would not allow execution of
// the provided safety-aware value.
func (thread *Thread) CheckPermits(value SafetyAware) error {
	if err := thread.requiredSafety.CheckValid(); err != nil {
		return fmt.Errorf("thread safety: %v", err)
	}
	safety := value.Safety()
	if err := safety.CheckValid(); err != nil {
		return err
	}
	return safety.CheckContains(thread.requiredSafety)
}

// Cancel causes execution of Starlark code in the specified thread to
// promptly fail with an EvalError that includes the specified reason.
// There may be a delay before the interpreter observes the cancellation
// if the thread is currently in a call to a built-in function.
//
// Cancellation cannot be undone.
//
// Unlike most methods of Thread, it is safe to call Cancel from any
// goroutine, even if the thread is actively executing.
func (thread *Thread) Cancel(reason string) {
	// Atomically set cancelReason, preserving earlier reason if any.
	atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&thread.cancelReason)), nil, unsafe.Pointer(&reason))
}

// SetLocal sets the thread-local value associated with the specified key.
// It must not be called after execution begins.
func (thread *Thread) SetLocal(key string, value interface{}) {
	if thread.locals == nil {
		thread.locals = make(map[string]interface{})
	}
	thread.locals[key] = value
}

// Local returns the thread-local value associated with the specified key.
func (thread *Thread) Local(key string) interface{} {
	return thread.locals[key]
}

// CallFrame returns a copy of the specified frame of the callstack.
// It should only be used in built-ins called from Starlark code.
// Depth 0 means the frame of the built-in itself, 1 is its caller, and so on.
//
// It is equivalent to CallStack().At(depth), but more efficient.
func (thread *Thread) CallFrame(depth int) CallFrame {
	return thread.frameAt(depth).asCallFrame()
}

// EnsureStack grows the stack to fit n more nested calls.
func (thread *Thread) EnsureStack(n int) {
	if n < 0 {
		panic("internal error: negative stack size")
	}

	newFrames := make([]frame, n)
	newStack := thread.stack
	for i := 0; i < n; i++ {
		newStack = append(newStack, &newFrames[i])
	}
	thread.stack = newStack[:len(thread.stack)]
}

func (thread *Thread) frameAt(depth int) *frame {
	return thread.stack[len(thread.stack)-1-depth]
}

// CallStack returns a new slice containing the thread's stack of call frames.
func (thread *Thread) CallStack() CallStack {
	frames := make([]CallFrame, len(thread.stack))
	for i, fr := range thread.stack {
		frames[i] = fr.asCallFrame()
	}
	return frames
}

// CallStackDepth returns the number of frames in the current call stack.
func (thread *Thread) CallStackDepth() int { return len(thread.stack) }

type StringBuilder interface {
	io.ByteWriter
	io.Writer
	io.StringWriter
	fmt.Stringer

	WriteRune(r rune) (size int, err error)
	Grow(n int)
	Cap() int
	Len() int
}

// SafeStringBuilder is a StringBuilder which is bound to a thread
// and which abides by sandboxing limits. Errors prevent subsequent
// operations.
type SafeStringBuilder struct {
	builder strings.Builder
	thread  *Thread
	allocs  uint64
	err     error
}

var _ StringBuilder = &SafeStringBuilder{}

// NewSafeStringBuilder returns a StringBuilder which abides by
// the sandbox limits of this thread.
func NewSafeStringBuilder(thread *Thread) *SafeStringBuilder {
	return &SafeStringBuilder{thread: thread}
}

// Allocs returns the total allocations reported to this SafeStringBuilder's
// thread.
func (tb *SafeStringBuilder) Allocs() uint64 {
	return tb.allocs
}

func (tb *SafeStringBuilder) safeGrow(n int) error {
	if tb.err != nil {
		return tb.err
	}

	if tb.Cap()-tb.Len() < n {
		// Make sure that we can allocate more
		newCap := tb.Cap()*2 + n
		newBufferSize := EstimateMakeSize([]byte{}, newCap)
		if err := tb.thread.AddAllocs(newBufferSize - int64(tb.allocs)); err != nil {
			tb.err = err
			return err
		}
		// The real size of the allocated buffer might be
		// bigger than expected. For this reason, add the
		// difference between the real buffer size and the
		// target capacity, so that every allocated byte
		// is available to the user.
		tb.builder.Grow(n + int(newBufferSize) - newCap)
		tb.allocs = uint64(newBufferSize)
	}
	return nil
}

func (tb *SafeStringBuilder) Grow(n int) {
	tb.safeGrow(n)
}

func (tb *SafeStringBuilder) Write(b []byte) (int, error) {
	if err := tb.safeGrow(len(b)); err != nil {
		return 0, err
	}

	return tb.builder.Write(b)
}

func (tb *SafeStringBuilder) WriteString(s string) (int, error) {
	if err := tb.safeGrow(len(s)); err != nil {
		return 0, err
	}

	return tb.builder.WriteString(s)
}

func (tb *SafeStringBuilder) WriteByte(b byte) error {
	if err := tb.safeGrow(1); err != nil {
		return err
	}

	return tb.builder.WriteByte(b)
}

func (tb *SafeStringBuilder) WriteRune(r rune) (int, error) {
	var growAmount int
	if r < utf8.RuneSelf {
		growAmount = 1
	} else {
		growAmount = utf8.UTFMax
	}
	if err := tb.safeGrow(growAmount); err != nil {
		return 0, err
	}

	return tb.builder.WriteRune(r)
}

func (tb *SafeStringBuilder) Cap() int       { return tb.builder.Cap() }
func (tb *SafeStringBuilder) Len() int       { return tb.builder.Len() }
func (tb *SafeStringBuilder) String() string { return tb.builder.String() }
func (tb *SafeStringBuilder) Err() error     { return tb.err }

// A StringDict is a mapping from names to values, and represents
// an environment such as the global variables of a module.
// It is not a true starlark.Value.
type StringDict map[string]Value

// Keys returns a new sorted slice of d's keys.
func (d StringDict) Keys() []string {
	names := make([]string, 0, len(d))
	for name := range d {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (d StringDict) String() string {
	buf := new(strings.Builder)
	buf.WriteByte('{')
	sep := ""
	for _, name := range d.Keys() {
		buf.WriteString(sep)
		buf.WriteString(name)
		buf.WriteString(": ")
		writeValue(buf, d[name], nil)
		sep = ", "
	}
	buf.WriteByte('}')
	return buf.String()
}

func (d StringDict) Freeze() {
	for _, v := range d {
		v.Freeze()
	}
}

// Has reports whether the dictionary contains the specified key.
func (d StringDict) Has(key string) bool { _, ok := d[key]; return ok }

// A frame records a call to a Starlark function (including module toplevel)
// or a built-in function or method.
type frame struct {
	callable  Callable // current function (or toplevel) or built-in
	pc        uint32   // program counter (Starlark frames only)
	locals    []Value  // local variables (Starlark frames only)
	spanStart int64    // start time of current profiler span
}

// Position returns the source position of the current point of execution in this frame.
func (fr *frame) Position() syntax.Position {
	switch c := fr.callable.(type) {
	case *Function:
		// Starlark function
		return c.funcode.Position(fr.pc)
	case callableWithPosition:
		// If a built-in Callable defines
		// a Position method, use it.
		return c.Position()
	}
	return syntax.MakePosition(&builtinFilename, 0, 0)
}

var builtinFilename = "<builtin>"

// Function returns the frame's function or built-in.
func (fr *frame) Callable() Callable { return fr.callable }

// A CallStack is a stack of call frames, outermost first.
type CallStack []CallFrame

// At returns a copy of the frame at depth i.
// At(0) returns the topmost frame.
func (stack CallStack) At(i int) CallFrame { return stack[len(stack)-1-i] }

// Pop removes and returns the topmost frame.
func (stack *CallStack) Pop() CallFrame {
	last := len(*stack) - 1
	top := (*stack)[last]
	*stack = (*stack)[:last]
	return top
}

// String returns a user-friendly description of the stack.
func (stack CallStack) String() string {
	out := new(strings.Builder)
	if len(stack) > 0 {
		fmt.Fprintf(out, "Traceback (most recent call last):\n")
	}
	for _, fr := range stack {
		fmt.Fprintf(out, "  %s: in %s\n", fr.Pos, fr.Name)
	}
	return out.String()
}

// An EvalError is a Starlark evaluation error and
// a copy of the thread's stack at the moment of the error.
type EvalError struct {
	Msg       string
	CallStack CallStack
	cause     error
}

// A CallFrame represents the function name and current
// position of execution of an enclosing call frame.
type CallFrame struct {
	Name string
	Pos  syntax.Position
}

func (fr *frame) asCallFrame() CallFrame {
	return CallFrame{
		Name: fr.Callable().Name(),
		Pos:  fr.Position(),
	}
}

func (thread *Thread) evalError(err error) *EvalError {
	return &EvalError{
		Msg:       err.Error(),
		CallStack: thread.CallStack(),
		cause:     err,
	}
}

func (e *EvalError) Error() string { return e.Msg }

// Backtrace returns a user-friendly error message describing the stack
// of calls that led to this error.
func (e *EvalError) Backtrace() string {
	// If the topmost stack frame is a built-in function,
	// remove it from the stack and add print "Error in fn:".
	stack := e.CallStack
	suffix := ""
	if last := len(stack) - 1; last >= 0 && stack[last].Pos.Filename() == builtinFilename {
		suffix = " in " + stack[last].Name
		stack = stack[:last]
	}
	return fmt.Sprintf("%sError%s: %s", stack, suffix, e.Msg)
}

func (e *EvalError) Unwrap() error { return e.cause }

// A Program is a compiled Starlark program.
//
// Programs are immutable, and contain no Values.
// A Program may be created by parsing a source file (see SourceProgram)
// or by loading a previously saved compiled program (see CompiledProgram).
type Program struct {
	compiled *compile.Program
}

// CompilerVersion is the version number of the protocol for compiled
// files. Applications must not run programs compiled by one version
// with an interpreter at another version, and should thus incorporate
// the compiler version into the cache key when reusing compiled code.
const CompilerVersion = compile.Version

// Filename returns the name of the file from which this program was loaded.
func (prog *Program) Filename() string { return prog.compiled.Toplevel.Pos.Filename() }

func (prog *Program) String() string { return prog.Filename() }

// NumLoads returns the number of load statements in the compiled program.
func (prog *Program) NumLoads() int { return len(prog.compiled.Loads) }

// Load(i) returns the name and position of the i'th module directly
// loaded by this one, where 0 <= i < NumLoads().
// The name is unresolved---exactly as it appears in the source.
func (prog *Program) Load(i int) (string, syntax.Position) {
	id := prog.compiled.Loads[i]
	return id.Name, id.Pos
}

// WriteTo writes the compiled module to the specified output stream.
func (prog *Program) Write(out io.Writer) error {
	data := prog.compiled.Encode()
	_, err := out.Write(data)
	return err
}

// ExecFile parses, resolves, and executes a Starlark file in the
// specified global environment, which may be modified during execution.
//
// Thread is the state associated with the Starlark thread.
//
// The filename and src parameters are as for syntax.Parse:
// filename is the name of the file to execute,
// and the name that appears in error messages;
// src is an optional source of bytes to use
// instead of filename.
//
// predeclared defines the predeclared names specific to this module.
// Execution does not modify this dictionary, though it may mutate
// its values.
//
// If ExecFile fails during evaluation, it returns an *EvalError
// containing a backtrace.
func ExecFile(thread *Thread, filename string, src interface{}, predeclared StringDict) (StringDict, error) {
	// Parse, resolve, and compile a Starlark source file.
	_, mod, err := SourceProgram(filename, src, predeclared.Has)
	if err != nil {
		return nil, err
	}

	g, err := mod.Init(thread, predeclared)
	g.Freeze()
	return g, err
}

// SourceProgram produces a new program by parsing, resolving,
// and compiling a Starlark source file.
// On success, it returns the parsed file and the compiled program.
// The filename and src parameters are as for syntax.Parse.
//
// The isPredeclared predicate reports whether a name is
// a pre-declared identifier of the current module.
// Its typical value is predeclared.Has,
// where predeclared is a StringDict of pre-declared values.
func SourceProgram(filename string, src interface{}, isPredeclared func(string) bool) (*syntax.File, *Program, error) {
	f, err := syntax.Parse(filename, src, 0)
	if err != nil {
		return nil, nil, err
	}
	prog, err := FileProgram(f, isPredeclared)
	return f, prog, err
}

// FileProgram produces a new program by resolving,
// and compiling the Starlark source file syntax tree.
// On success, it returns the compiled program.
//
// Resolving a syntax tree mutates it.
// Do not call FileProgram more than once on the same file.
//
// The isPredeclared predicate reports whether a name is
// a pre-declared identifier of the current module.
// Its typical value is predeclared.Has,
// where predeclared is a StringDict of pre-declared values.
func FileProgram(f *syntax.File, isPredeclared func(string) bool) (*Program, error) {
	if err := resolve.File(f, isPredeclared, Universe.Has); err != nil {
		return nil, err
	}

	var pos syntax.Position
	if len(f.Stmts) > 0 {
		pos = syntax.Start(f.Stmts[0])
	} else {
		pos = syntax.MakePosition(&f.Path, 1, 1)
	}

	module := f.Module.(*resolve.Module)
	compiled := compile.File(f.Stmts, pos, "<toplevel>", module.Locals, module.Globals)

	return &Program{compiled}, nil
}

// CompiledProgram produces a new program from the representation
// of a compiled program previously saved by Program.Write.
func CompiledProgram(in io.Reader) (*Program, error) {
	data, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, err
	}
	compiled, err := compile.DecodeProgram(data)
	if err != nil {
		return nil, err
	}
	return &Program{compiled}, nil
}

// Init creates a set of global variables for the program,
// executes the toplevel code of the specified program,
// and returns a new, unfrozen dictionary of the globals.
func (prog *Program) Init(thread *Thread, predeclared StringDict) (StringDict, error) {
	toplevel := makeToplevelFunction(prog.compiled, predeclared)

	_, err := Call(thread, toplevel, nil, nil)

	// Convert the global environment to a map.
	// We return a (partial) map even in case of error.
	return toplevel.Globals(), err
}

// ExecREPLChunk compiles and executes file f in the specified thread
// and global environment. This is a variant of ExecFile specialized to
// the needs of a REPL, in which a sequence of input chunks, each
// syntactically a File, manipulates the same set of module globals,
// which are not frozen after execution.
//
// This function is intended to support only github.com/canonical/starlark/repl.
// Its API stability is not guaranteed.
func ExecREPLChunk(f *syntax.File, thread *Thread, globals StringDict) error {
	var predeclared StringDict

	// -- variant of FileProgram --

	if err := resolve.REPLChunk(f, globals.Has, predeclared.Has, Universe.Has); err != nil {
		return err
	}

	var pos syntax.Position
	if len(f.Stmts) > 0 {
		pos = syntax.Start(f.Stmts[0])
	} else {
		pos = syntax.MakePosition(&f.Path, 1, 1)
	}

	module := f.Module.(*resolve.Module)
	compiled := compile.File(f.Stmts, pos, "<toplevel>", module.Locals, module.Globals)
	prog := &Program{compiled}

	// -- variant of Program.Init --

	toplevel := makeToplevelFunction(prog.compiled, predeclared)

	// Initialize module globals from parameter.
	for i, id := range prog.compiled.Globals {
		if v := globals[id.Name]; v != nil {
			toplevel.module.globals[i] = v
		}
	}

	_, err := Call(thread, toplevel, nil, nil)

	// Reflect changes to globals back to parameter, even after an error.
	for i, id := range prog.compiled.Globals {
		if v := toplevel.module.globals[i]; v != nil {
			globals[id.Name] = v
		}
	}

	return err
}

func makeToplevelFunction(prog *compile.Program, predeclared StringDict) *Function {
	// Create the Starlark value denoted by each program constant c.
	constants := make([]Value, len(prog.Constants))
	for i, c := range prog.Constants {
		var v Value
		switch c := c.(type) {
		case int64:
			v = MakeInt64(c)
		case *big.Int:
			v = MakeBigInt(c)
		case string:
			v = String(c)
		case compile.Bytes:
			v = Bytes(c)
		case float64:
			v = Float(c)
		default:
			log.Panicf("unexpected constant %T: %v", c, c)
		}
		constants[i] = v
	}

	return &Function{
		funcode: prog.Toplevel,
		module: &module{
			program:     prog,
			predeclared: predeclared,
			globals:     make([]Value, len(prog.Globals)),
			constants:   constants,
		},
	}
}

// Eval parses, resolves, and evaluates an expression within the
// specified (predeclared) environment.
//
// Evaluation cannot mutate the environment dictionary itself,
// though it may modify variables reachable from the dictionary.
//
// The filename and src parameters are as for syntax.Parse.
//
// If Eval fails during evaluation, it returns an *EvalError
// containing a backtrace.
func Eval(thread *Thread, filename string, src interface{}, env StringDict) (Value, error) {
	expr, err := syntax.ParseExpr(filename, src, 0)
	if err != nil {
		return nil, err
	}
	f, err := makeExprFunc(expr, env)
	if err != nil {
		return nil, err
	}
	return Call(thread, f, nil, nil)
}

// EvalExpr resolves and evaluates an expression within the
// specified (predeclared) environment.
// Evaluating a comma-separated list of expressions yields a tuple value.
//
// Resolving an expression mutates it.
// Do not call EvalExpr more than once for the same expression.
//
// Evaluation cannot mutate the environment dictionary itself,
// though it may modify variables reachable from the dictionary.
//
// If Eval fails during evaluation, it returns an *EvalError
// containing a backtrace.
func EvalExpr(thread *Thread, expr syntax.Expr, env StringDict) (Value, error) {
	fn, err := makeExprFunc(expr, env)
	if err != nil {
		return nil, err
	}
	return Call(thread, fn, nil, nil)
}

// ExprFunc returns a no-argument function
// that evaluates the expression whose source is src.
func ExprFunc(filename string, src interface{}, env StringDict) (*Function, error) {
	expr, err := syntax.ParseExpr(filename, src, 0)
	if err != nil {
		return nil, err
	}
	return makeExprFunc(expr, env)
}

// makeExprFunc returns a no-argument function whose body is expr.
func makeExprFunc(expr syntax.Expr, env StringDict) (*Function, error) {
	locals, err := resolve.Expr(expr, env.Has, Universe.Has)
	if err != nil {
		return nil, err
	}

	return makeToplevelFunction(compile.Expr(expr, "<expr>", locals), env), nil
}

// The following functions are primitive operations of the byte code interpreter.

// list += iterable
func safeListExtend(thread *Thread, x *List, y Iterable) error {
	elemsAppender := NewSafeAppender(thread, &x.elems)
	if ylist, ok := y.(*List); ok {
		// fast path: list += list
		if err := elemsAppender.AppendSlice(ylist.elems); err != nil {
			return err
		}
	} else {
		iter, err := SafeIterate(thread, y)
		if err != nil {
			return err
		}
		defer iter.Done()
		var z Value
		for iter.Next(&z) {
			if err := elemsAppender.Append(z); err != nil {
				return err
			}
		}
		if err := iter.Err(); err != nil {
			return err
		}
	}
	return nil
}

// getAttr implements x.dot.
func getAttr(x Value, name string) (Value, error) {
	hasAttr, ok := x.(HasAttrs)
	if !ok {
		return nil, fmt.Errorf("%s has no .%s field or method", x.Type(), name)
	}

	var errmsg string
	v, err := hasAttr.Attr(name)
	if err == nil {
		if v != nil {
			return v, nil // success
		}
		// (nil, nil) => generic error
		errmsg = fmt.Sprintf("%s has no .%s field or method", x.Type(), name)
	} else if nsa, ok := err.(NoSuchAttrError); ok {
		errmsg = string(nsa)
	} else {
		return nil, err // return error as is
	}

	// add spelling hint
	if n := spell.Nearest(name, hasAttr.AttrNames()); n != "" {
		errmsg = fmt.Sprintf("%s (did you mean .%s?)", errmsg, n)
	}

	return nil, fmt.Errorf("%s", errmsg)
}

// setField implements x.name = y.
func setField(x Value, name string, y Value) error {
	if x, ok := x.(HasSetField); ok {
		err := x.SetField(name, y)
		if _, ok := err.(NoSuchAttrError); ok {
			// No such field: check spelling.
			if n := spell.Nearest(name, x.AttrNames()); n != "" {
				err = fmt.Errorf("%s (did you mean .%s?)", err, n)
			}
		}
		return err
	}

	return fmt.Errorf("can't assign to .%s field of %s", name, x.Type())
}

// getIndex implements x[y].
func getIndex(x, y Value) (Value, error) {
	switch x := x.(type) {
	case Mapping: // dict
		z, found, err := x.Get(y)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("key %v not in %s", y, x.Type())
		}
		return z, nil

	case Indexable: // string, list, tuple
		n := x.Len()
		i, err := AsInt32(y)
		if err != nil {
			return nil, fmt.Errorf("%s index: %s", x.Type(), err)
		}
		origI := i
		if i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return nil, outOfRange(origI, n, x)
		}
		return x.Index(i), nil
	}
	return nil, fmt.Errorf("unhandled index operation %s[%s]", x.Type(), y.Type())
}

func outOfRange(i, n int, x Value) error {
	if n == 0 {
		return fmt.Errorf("index %d out of range: empty %s", i, x.Type())
	} else {
		return fmt.Errorf("%s index %d out of range [%d:%d]", x.Type(), i, -n, n-1)
	}
}

func sanitizeIndex(collection Indexable, i int) (int, error) {
	n := collection.Len()
	origI := i
	if i < 0 {
		i += n
	}
	if i < 0 || i >= n {
		return 0, outOfRange(origI, n, collection)
	}
	return i, nil
}

// setIndex implements x[y] = z.
func setIndex(thread *Thread, x, y, z Value) error {
	switch x := x.(type) {
	case HasSafeSetKey:
		return x.SafeSetKey(thread, y, z)

	case HasSafeSetIndex:
		i, err := AsInt32(y)
		if err != nil {
			return err
		}

		if i, err = sanitizeIndex(x, i); err != nil {
			return err
		}
		return x.SafeSetIndex(thread, i, z)

	case HasSetKey:
		if err := CheckSafety(thread, NotSafe); err != nil {
			return err
		}
		return x.SetKey(y, z)

	case HasSetIndex:
		if err := CheckSafety(thread, NotSafe); err != nil {
			return err
		}
		i, err := AsInt32(y)
		if err != nil {
			return err
		}
		if i, err = sanitizeIndex(x, i); err != nil {
			return err
		}
		return x.SetIndex(i, z)

	default:
		return fmt.Errorf("%s value does not support item assignment", x.Type())
	}
}

// Unary applies a unary operator (+, -, ~, not) to its operand.
func Unary(op syntax.Token, x Value) (Value, error) {
	// The NOT operator is not customizable.
	if op == syntax.NOT {
		return !x.Truth(), nil
	}

	// Int, Float, and user-defined types
	if x, ok := x.(HasUnary); ok {
		// (nil, nil) => unhandled
		y, err := x.Unary(op)
		if y != nil || err != nil {
			return y, err
		}
	}

	return nil, fmt.Errorf("unknown unary op: %s %s", op, x.Type())
}

// SafeBinary applies a strict binary operator (not AND or OR) to its operands,
// respecting safety.
func SafeBinary(thread *Thread, op syntax.Token, x, y Value) (Value, error) {
	return safeBinary(thread, op, x, y)
}

// Binary applies a strict binary operator (not AND or OR) to its operands.
// For equality tests or ordered comparisons, use Compare instead.
func Binary(op syntax.Token, x, y Value) (Value, error) {
	return safeBinary(nil, op, x, y)
}

var floatSize = EstimateSize(Float(0))

func safeBinary(thread *Thread, op syntax.Token, x, y Value) (Value, error) {
	if err := CheckSafety(thread, MemSafe); err != nil {
		return nil, err
	}

	max := func(a, b int64) int64 {
		if a > b {
			return a
		}
		return b
	}
	switch op {
	case syntax.PLUS:
		switch x := x.(type) {
		case String:
			if y, ok := y.(String); ok {
				if thread != nil {
					resultSize := EstimateMakeSize([]byte{}, len(x)+len(y)) + StringTypeOverhead
					if err := thread.AddAllocs(resultSize); err != nil {
						return nil, err
					}
				}
				return x + y, nil
			}
		case Int:
			switch y := y.(type) {
			case Int:
				if thread != nil {
					if err := thread.CheckAllocs(max(EstimateSize(x), EstimateSize(y))); err != nil {
						return nil, err
					}
					result := Value(x.Add(y))
					if err := thread.AddAllocs(EstimateSize(result)); err != nil {
						return nil, err
					}
					return result, nil
				}
				return x.Add(y), nil
			case Float:
				xf, err := x.finiteFloat()
				if err != nil {
					return nil, err
				}
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return xf + y, nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return x + y, nil
			case Int:
				yf, err := y.finiteFloat()
				if err != nil {
					return nil, err
				}
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return x + yf, nil
			}
		case *List:
			if y, ok := y.(*List); ok {
				if thread != nil {
					resultSize := EstimateMakeSize([]Value{}, x.Len()+y.Len()) + EstimateSize(&List{})
					if err := thread.AddAllocs(resultSize); err != nil {
						return nil, err
					}
				}
				z := make([]Value, 0, x.Len()+y.Len())
				z = append(z, x.elems...)
				z = append(z, y.elems...)
				return NewList(z), nil
			}
		case Tuple:
			if y, ok := y.(Tuple); ok {
				if thread != nil {
					zSize := EstimateMakeSize(Tuple{}, len(x)+len(y)) + SliceTypeOverhead
					if err := thread.AddAllocs(zSize); err != nil {
						return nil, err
					}
				}
				z := make(Tuple, 0, len(x)+len(y))
				z = append(z, x...)
				z = append(z, y...)
				return z, nil
			}
		}

	case syntax.MINUS:
		switch x := x.(type) {
		case Int:
			switch y := y.(type) {
			case Int:
				return x.Sub(y), nil
			case Float:
				xf, err := x.finiteFloat()
				if err != nil {
					return nil, err
				}
				return xf - y, nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				return x - y, nil
			case Int:
				yf, err := y.finiteFloat()
				if err != nil {
					return nil, err
				}
				return x - yf, nil
			}
		}

	case syntax.STAR:
		switch x := x.(type) {
		case Int:
			switch y := y.(type) {
			case Int:
				if thread != nil {
					if err := thread.CheckAllocs(EstimateSize(x) + EstimateSize(y)); err != nil {
						return nil, err
					}
					result := Value(x.Mul(y))
					if err := thread.AddAllocs(EstimateSize(result)); err != nil {
						return nil, err
					}
					return result, nil
				}
				return x.Mul(y), nil
			case Float:
				xf, err := x.finiteFloat()
				if err != nil {
					return nil, err
				}
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(floatSize)); err != nil {
						return nil, err
					}
				}
				return xf * y, nil
			case String:
				if thread != nil {
					if err := thread.AddAllocs(StringTypeOverhead); err != nil {
						return nil, err
					}
				}
				return stringRepeat(thread, y, x)
			case Bytes:
				if thread != nil {
					if err := thread.AddAllocs(StringTypeOverhead); err != nil {
						return nil, err
					}
				}
				return bytesRepeat(thread, y, x)
			case *List:
				elems, err := tupleRepeat(thread, Tuple(y.elems), x)
				if err != nil {
					return nil, err
				}
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(&List{})); err != nil {
						return nil, err
					}
				}
				return NewList(elems), nil
			case Tuple:
				if thread != nil {
					if err := thread.AddAllocs(SliceTypeOverhead); err != nil {
						return nil, err
					}
				}
				return tupleRepeat(thread, y, x)
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(floatSize)); err != nil {
						return nil, err
					}
				}
				return x * y, nil
			case Int:
				yf, err := y.finiteFloat()
				if err != nil {
					return nil, err
				}
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(floatSize)); err != nil {
						return nil, err
					}
				}
				return x * yf, nil
			}
		case String:
			if y, ok := y.(Int); ok {
				if thread != nil {
					if err := thread.AddAllocs(StringTypeOverhead); err != nil {
						return nil, err
					}
				}
				return stringRepeat(thread, x, y)
			}
		case Bytes:
			if y, ok := y.(Int); ok {
				if thread != nil {
					if err := thread.AddAllocs(StringTypeOverhead); err != nil {
						return nil, err
					}
				}
				return bytesRepeat(thread, x, y)
			}
		case *List:
			if y, ok := y.(Int); ok {
				elems, err := tupleRepeat(thread, Tuple(x.elems), y)
				if err != nil {
					return nil, err
				}
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(&List{})); err != nil {
						return nil, err
					}
				}
				return NewList(elems), nil
			}
		case Tuple:
			if y, ok := y.(Int); ok {
				if thread != nil {
					if err := thread.AddAllocs(SliceTypeOverhead); err != nil {
						return nil, err
					}
				}
				return tupleRepeat(thread, x, y)
			}

		}

	case syntax.SLASH:
		switch x := x.(type) {
		case Int:
			xf, err := x.finiteFloat()
			if err != nil {
				return nil, err
			}
			switch y := y.(type) {
			case Int:
				yf, err := y.finiteFloat()
				if err != nil {
					return nil, err
				}
				if yf == 0.0 {
					return nil, fmt.Errorf("floating-point division by zero")
				}
				return xf / yf, nil
			case Float:
				if y == 0.0 {
					return nil, fmt.Errorf("floating-point division by zero")
				}
				return xf / y, nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if y == 0.0 {
					return nil, fmt.Errorf("floating-point division by zero")
				}
				return x / y, nil
			case Int:
				yf, err := y.finiteFloat()
				if err != nil {
					return nil, err
				}
				if yf == 0.0 {
					return nil, fmt.Errorf("floating-point division by zero")
				}
				return x / yf, nil
			}
		}

	case syntax.SLASHSLASH:
		switch x := x.(type) {
		case Int:
			switch y := y.(type) {
			case Int:
				if y.Sign() == 0 {
					return nil, fmt.Errorf("floored division by zero")
				}
				return x.Div(y), nil
			case Float:
				xf, err := x.finiteFloat()
				if err != nil {
					return nil, err
				}
				if y == 0.0 {
					return nil, fmt.Errorf("floored division by zero")
				}
				return floor(xf / y), nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if y == 0.0 {
					return nil, fmt.Errorf("floored division by zero")
				}
				return floor(x / y), nil
			case Int:
				yf, err := y.finiteFloat()
				if err != nil {
					return nil, err
				}
				if yf == 0.0 {
					return nil, fmt.Errorf("floored division by zero")
				}
				return floor(x / yf), nil
			}
		}

	case syntax.PERCENT:
		switch x := x.(type) {
		case Int:
			switch y := y.(type) {
			case Int:
				if y.Sign() == 0 {
					return nil, fmt.Errorf("integer modulo by zero")
				}
				return x.Mod(y), nil
			case Float:
				xf, err := x.finiteFloat()
				if err != nil {
					return nil, err
				}
				if y == 0 {
					return nil, fmt.Errorf("floating-point modulo by zero")
				}
				return xf.Mod(y), nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if y == 0.0 {
					return nil, fmt.Errorf("floating-point modulo by zero")
				}
				return x.Mod(y), nil
			case Int:
				if y.Sign() == 0 {
					return nil, fmt.Errorf("floating-point modulo by zero")
				}
				yf, err := y.finiteFloat()
				if err != nil {
					return nil, err
				}
				return x.Mod(yf), nil
			}
		case String:
			return interpolate(string(x), y)
		}

	case syntax.NOT_IN:
		z, err := Binary(syntax.IN, x, y)
		if err != nil {
			return nil, err
		}
		return !z.Truth(), nil

	case syntax.IN:
		switch y := y.(type) {
		case *List:
			for _, elem := range y.elems {
				if eq, err := Equal(elem, x); err != nil {
					return nil, err
				} else if eq {
					return True, nil
				}
			}
			return False, nil
		case Tuple:
			for _, elem := range y {
				if eq, err := Equal(elem, x); err != nil {
					return nil, err
				} else if eq {
					return True, nil
				}
			}
			return False, nil
		case Mapping: // e.g. dict
			// Ignore error from Get as we cannot distinguish true
			// errors (value cycle, type error) from "key not found".
			_, found, _ := y.Get(x)
			return Bool(found), nil
		case *Set:
			ok, err := y.Has(x)
			return Bool(ok), err
		case String:
			needle, ok := x.(String)
			if !ok {
				return nil, fmt.Errorf("'in <string>' requires string as left operand, not %s", x.Type())
			}
			return Bool(strings.Contains(string(y), string(needle))), nil
		case Bytes:
			switch needle := x.(type) {
			case Bytes:
				return Bool(strings.Contains(string(y), string(needle))), nil
			case Int:
				var b byte
				if err := AsInt(needle, &b); err != nil {
					return nil, fmt.Errorf("int in bytes: %s", err)
				}
				return Bool(strings.IndexByte(string(y), b) >= 0), nil
			default:
				return nil, fmt.Errorf("'in bytes' requires bytes or int as left operand, not %s", x.Type())
			}
		case rangeValue:
			i, err := NumberToInt(x)
			if err != nil {
				return nil, fmt.Errorf("'in <range>' requires integer as left operand, not %s", x.Type())
			}
			return Bool(y.contains(i)), nil
		}

	case syntax.PIPE:
		switch x := x.(type) {
		case Int:
			if y, ok := y.(Int); ok {
				return x.Or(y), nil
			}
		case *Set: // union
			if y, ok := y.(*Set); ok {
				// TODO: use SafeIterate
				iter := Iterate(y)
				defer iter.Done()
				return x.Union(iter)
			}
		}

	case syntax.AMP:
		switch x := x.(type) {
		case Int:
			if y, ok := y.(Int); ok {
				return x.And(y), nil
			}
		case *Set: // intersection
			if y, ok := y.(*Set); ok {
				set := new(Set)
				if x.Len() > y.Len() {
					x, y = y, x // opt: range over smaller set
				}
				for _, xelem := range x.elems() {
					// Has, Insert cannot fail here.
					if found, _ := y.Has(xelem); found {
						set.Insert(xelem)
					}
				}
				return set, nil
			}
		}

	case syntax.CIRCUMFLEX:
		switch x := x.(type) {
		case Int:
			if y, ok := y.(Int); ok {
				return x.Xor(y), nil
			}
		case *Set: // symmetric difference
			if y, ok := y.(*Set); ok {
				set := new(Set)
				for _, xelem := range x.elems() {
					if found, _ := y.Has(xelem); !found {
						set.Insert(xelem)
					}
				}
				for _, yelem := range y.elems() {
					if found, _ := x.Has(yelem); !found {
						set.Insert(yelem)
					}
				}
				return set, nil
			}
		}

	case syntax.LTLT, syntax.GTGT:
		if x, ok := x.(Int); ok {
			y, err := AsInt32(y)
			if err != nil {
				return nil, err
			}
			if y < 0 {
				return nil, fmt.Errorf("negative shift count: %v", y)
			}
			if op == syntax.LTLT {
				if y >= 512 {
					return nil, fmt.Errorf("shift count too large: %v", y)
				}
				return x.Lsh(uint(y)), nil
			} else {
				return x.Rsh(uint(y)), nil
			}
		}

	default:
		// unknown operator
		goto unknown
	}

	// user-defined types
	// (nil, nil) => unhandled
	// TODO: use SafeIterate (SafeBinary?)
	if err := CheckSafety(thread, NotSafe); err != nil {
		return nil, err
	}
	if x, ok := x.(HasBinary); ok {
		z, err := x.Binary(op, y, Left)
		if z != nil || err != nil {
			return z, err
		}
	}
	if y, ok := y.(HasBinary); ok {
		z, err := y.Binary(op, x, Right)
		if z != nil || err != nil {
			return z, err
		}
	}

	// unsupported operand types
unknown:
	return nil, fmt.Errorf("unknown binary op: %s %s %s", x.Type(), op, y.Type())
}

// It's always possible to overeat in small bites but we'll
// try to stop someone swallowing the world in one gulp.
const maxAlloc = 1 << 30

func tupleRepeat(thread *Thread, elems Tuple, n Int) (Tuple, error) {
	if len(elems) == 0 {
		return nil, nil
	}
	i, err := AsInt32(n)
	if err != nil {
		return nil, fmt.Errorf("repeat count %s too large", n)
	}
	if i < 1 {
		return nil, nil
	}
	// Inv: i > 0, len > 0
	sz := len(elems) * i
	if sz < 0 || sz >= maxAlloc { // sz < 0 => overflow
		// Don't print sz.
		return nil, fmt.Errorf("excessive repeat (%d * %d elements)", len(elems), i)
	}
	if thread != nil {
		if err := thread.AddAllocs(EstimateMakeSize([]Value{}, sz)); err != nil {
			return nil, err
		}
	}
	res := make([]Value, sz)
	// copy elems into res, doubling each time
	x := copy(res, elems)
	for x < len(res) {
		copy(res[x:], res[:x])
		x *= 2
	}
	return res, nil
}

func bytesRepeat(thread *Thread, b Bytes, n Int) (Bytes, error) {
	res, err := stringRepeat(thread, String(b), n)
	return Bytes(res), err
}

func stringRepeat(thread *Thread, s String, n Int) (String, error) {
	if s == "" {
		return "", nil
	}
	i, err := AsInt32(n)
	if err != nil {
		return "", fmt.Errorf("repeat count %s too large", n)
	}
	if i < 1 {
		return "", nil
	}
	// Inv: i > 0, len > 0
	sz := len(s) * i
	if sz < 0 || sz >= maxAlloc { // sz < 0 => overflow
		// Don't print sz.
		return "", fmt.Errorf("excessive repeat (%d * %d elements)", len(s), i)
	}
	if thread != nil {
		if err := thread.AddAllocs(EstimateMakeSize([]byte{}, sz)); err != nil {
			return "", err
		}
	}
	return String(strings.Repeat(string(s), i)), nil
}

// Call calls the function fn with the specified positional and keyword arguments.
func Call(thread *Thread, fn Value, args Tuple, kwargs []Tuple) (Value, error) {
	c, ok := fn.(Callable)
	if !ok {
		return nil, fmt.Errorf("invalid call of non-function (%s)", fn.Type())
	}

	// Check safety flags
	callableSafety := NotSafe
	if c, ok := c.(SafetyAware); ok {
		callableSafety = c.Safety()
	}
	if err := thread.CheckPermits(callableSafety); err != nil {
		if _, ok := c.(*Function); ok {
			return nil, err
		}
		if b, ok := c.(*Builtin); ok {
			return nil, fmt.Errorf("cannot call builtin '%s': %v", b.Name(), err)
		}
		return nil, fmt.Errorf("cannot call value of type '%s': %v", c.Type(), err)
	}

	// Allocate and push a new frame.
	var fr *frame
	// Optimization: use slack portion of thread.stack
	// slice as a freelist of empty frames.
	if n := len(thread.stack); n < cap(thread.stack) {
		fr = thread.stack[n : n+1][0]
	}
	if fr == nil {
		fr = new(frame)
	}

	// one-time initialization of thread
	if thread.maxSteps == 0 {
		thread.maxSteps-- // (MaxUint64)
	}

	thread.stack = append(thread.stack, fr) // push

	fr.callable = c

	thread.beginProfSpan()
	result, err := c.CallInternal(thread, args, kwargs)
	thread.endProfSpan()

	// Sanity check: nil is not a valid Starlark value.
	if result == nil && err == nil {
		err = fmt.Errorf("internal error: nil (not None) returned from %s", fn)
	}

	// Always return an EvalError with an accurate frame.
	if err != nil {
		if _, ok := err.(*EvalError); !ok {
			err = thread.evalError(err)
		}
	}

	*fr = frame{}                                     // clear out any references
	thread.stack = thread.stack[:len(thread.stack)-1] // pop

	return result, err
}

func slice(x, lo, hi, step_ Value) (Value, error) {
	sliceable, ok := x.(Sliceable)
	if !ok {
		return nil, fmt.Errorf("invalid slice operand %s", x.Type())
	}

	n := sliceable.Len()
	step := 1
	if step_ != None {
		var err error
		step, err = AsInt32(step_)
		if err != nil {
			return nil, fmt.Errorf("invalid slice step: %s", err)
		}
		if step == 0 {
			return nil, fmt.Errorf("zero is not a valid slice step")
		}
	}

	// TODO(adonovan): opt: preallocate result array.

	var start, end int
	if step > 0 {
		// positive stride
		// default indices are [0:n].
		var err error
		start, end, err = indices(lo, hi, n)
		if err != nil {
			return nil, err
		}

		if end < start {
			end = start // => empty result
		}
	} else {
		// negative stride
		// default indices are effectively [n-1:-1], though to
		// get this effect using explicit indices requires
		// [n-1:-1-n:-1] because of the treatment of -ve values.
		start = n - 1
		if err := asIndex(lo, n, &start); err != nil {
			return nil, fmt.Errorf("invalid start index: %s", err)
		}
		if start >= n {
			start = n - 1
		}

		end = -1
		if err := asIndex(hi, n, &end); err != nil {
			return nil, fmt.Errorf("invalid end index: %s", err)
		}
		if end < -1 {
			end = -1
		}

		if start < end {
			start = end // => empty result
		}
	}

	return sliceable.Slice(start, end, step), nil
}

// From Hacker's Delight, section 2.8.
func signum64(x int64) int { return int(uint64(x>>63) | uint64(-x)>>63) }
func signum(x int) int     { return signum64(int64(x)) }

// indices converts start_ and end_ to indices in the range [0:len].
// The start index defaults to 0 and the end index defaults to len.
// An index -len < i < 0 is treated like i+len.
// All other indices outside the range are clamped to the nearest value in the range.
// Beware: start may be greater than end.
// This function is suitable only for slices with positive strides.
func indices(start_, end_ Value, len int) (start, end int, err error) {
	start = 0
	if err := asIndex(start_, len, &start); err != nil {
		return 0, 0, fmt.Errorf("invalid start index: %s", err)
	}
	// Clamp to [0:len].
	if start < 0 {
		start = 0
	} else if start > len {
		start = len
	}

	end = len
	if err := asIndex(end_, len, &end); err != nil {
		return 0, 0, fmt.Errorf("invalid end index: %s", err)
	}
	// Clamp to [0:len].
	if end < 0 {
		end = 0
	} else if end > len {
		end = len
	}

	return start, end, nil
}

// asIndex sets *result to the integer value of v, adding len to it
// if it is negative.  If v is nil or None, *result is unchanged.
func asIndex(v Value, len int, result *int) error {
	if v != nil && v != None {
		var err error
		*result, err = AsInt32(v)
		if err != nil {
			return err
		}
		if *result < 0 {
			*result += len
		}
	}
	return nil
}

// setArgs sets the values of the formal parameters of function fn in
// based on the actual parameter values in args and kwargs.
func setArgs(locals []Value, fn *Function, args Tuple, kwargs []Tuple) error {

	// This is the general schema of a function:
	//
	//   def f(p1, p2=dp2, p3=dp3, *args, k1, k2=dk2, k3, **kwargs)
	//
	// The p parameters are non-kwonly, and may be specified positionally.
	// The k parameters are kwonly, and must be specified by name.
	// The defaults tuple is (dp2, dp3, mandatory, dk2, mandatory).
	//
	// Arguments are processed as follows:
	// - positional arguments are bound to a prefix of [p1, p2, p3].
	// - surplus positional arguments are bound to *args.
	// - keyword arguments are bound to any of {p1, p2, p3, k1, k2, k3};
	//   duplicate bindings are rejected.
	// - surplus keyword arguments are bound to **kwargs.
	// - defaults are bound to each parameter from p2 to k3 if no value was set.
	//   default values come from the tuple above.
	//   It is an error if the tuple entry for an unset parameter is 'mandatory'.

	// Nullary function?
	if fn.NumParams() == 0 {
		if nactual := len(args) + len(kwargs); nactual > 0 {
			return fmt.Errorf("function %s accepts no arguments (%d given)", fn.Name(), nactual)
		}
		return nil
	}

	cond := func(x bool, y, z interface{}) interface{} {
		if x {
			return y
		}
		return z
	}

	// nparams is the number of ordinary parameters (sans *args and **kwargs).
	nparams := fn.NumParams()
	var kwdict *Dict
	if fn.HasKwargs() {
		nparams--
		kwdict = new(Dict)
		locals[nparams] = kwdict
	}
	if fn.HasVarargs() {
		nparams--
	}

	// nonkwonly is the number of non-kwonly parameters.
	nonkwonly := nparams - fn.NumKwonlyParams()

	// Too many positional args?
	n := len(args)
	if len(args) > nonkwonly {
		if !fn.HasVarargs() {
			return fmt.Errorf("function %s accepts %s%d positional argument%s (%d given)",
				fn.Name(),
				cond(len(fn.defaults) > fn.NumKwonlyParams(), "at most ", ""),
				nonkwonly,
				cond(nonkwonly == 1, "", "s"),
				len(args))
		}
		n = nonkwonly
	}

	// Bind positional arguments to non-kwonly parameters.
	for i := 0; i < n; i++ {
		locals[i] = args[i]
	}

	// Bind surplus positional arguments to *args parameter.
	if fn.HasVarargs() {
		tuple := make(Tuple, len(args)-n)
		for i := n; i < len(args); i++ {
			tuple[i-n] = args[i]
		}
		locals[nparams] = tuple
	}

	// Bind keyword arguments to parameters.
	paramIdents := fn.funcode.Locals[:nparams]
	for _, pair := range kwargs {
		k, v := pair[0].(String), pair[1]
		if i := findParam(paramIdents, string(k)); i >= 0 {
			if locals[i] != nil {
				return fmt.Errorf("function %s got multiple values for parameter %s", fn.Name(), k)
			}
			locals[i] = v
			continue
		}
		if kwdict == nil {
			return fmt.Errorf("function %s got an unexpected keyword argument %s", fn.Name(), k)
		}
		oldlen := kwdict.Len()
		kwdict.SetKey(k, v)
		if kwdict.Len() == oldlen {
			return fmt.Errorf("function %s got multiple values for parameter %s", fn.Name(), k)
		}
	}

	// Are defaults required?
	if n < nparams || fn.NumKwonlyParams() > 0 {
		m := nparams - len(fn.defaults) // first default

		// Report errors for missing required arguments.
		var missing []string
		var i int
		for i = n; i < m; i++ {
			if locals[i] == nil {
				missing = append(missing, paramIdents[i].Name)
			}
		}

		// Bind default values to parameters.
		for ; i < nparams; i++ {
			if locals[i] == nil {
				dflt := fn.defaults[i-m]
				if _, ok := dflt.(mandatory); ok {
					missing = append(missing, paramIdents[i].Name)
					continue
				}
				locals[i] = dflt
			}
		}

		if missing != nil {
			return fmt.Errorf("function %s missing %d argument%s (%s)",
				fn.Name(), len(missing), cond(len(missing) > 1, "s", ""), strings.Join(missing, ", "))
		}
	}
	return nil
}

func findParam(params []compile.Binding, name string) int {
	for i, param := range params {
		if param.Name == name {
			return i
		}
	}
	return -1
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#string-interpolation
func interpolate(format string, x Value) (Value, error) {
	buf := new(strings.Builder)
	index := 0
	nargs := 1
	if tuple, ok := x.(Tuple); ok {
		nargs = len(tuple)
	}
	for {
		i := strings.IndexByte(format, '%')
		if i < 0 {
			buf.WriteString(format)
			break
		}
		buf.WriteString(format[:i])
		format = format[i+1:]

		if format != "" && format[0] == '%' {
			buf.WriteByte('%')
			format = format[1:]
			continue
		}

		var arg Value
		if format != "" && format[0] == '(' {
			// keyword argument: %(name)s.
			format = format[1:]
			j := strings.IndexByte(format, ')')
			if j < 0 {
				return nil, fmt.Errorf("incomplete format key")
			}
			key := format[:j]
			if dict, ok := x.(Mapping); !ok {
				return nil, fmt.Errorf("format requires a mapping")
			} else if v, found, _ := dict.Get(String(key)); found {
				arg = v
			} else {
				return nil, fmt.Errorf("key not found: %s", key)
			}
			format = format[j+1:]
		} else {
			// positional argument: %s.
			if index >= nargs {
				return nil, fmt.Errorf("not enough arguments for format string")
			}
			if tuple, ok := x.(Tuple); ok {
				arg = tuple[index]
			} else {
				arg = x
			}
		}

		// NOTE: Starlark does not support any of these optional Python features:
		// - optional conversion flags: [#0- +], etc.
		// - optional minimum field width (number or *).
		// - optional precision (.123 or *)
		// - optional length modifier

		// conversion type
		if format == "" {
			return nil, fmt.Errorf("incomplete format")
		}
		switch c := format[0]; c {
		case 's', 'r':
			if str, ok := AsString(arg); ok && c == 's' {
				buf.WriteString(str)
			} else {
				writeValue(buf, arg, nil)
			}
		case 'd', 'i', 'o', 'x', 'X':
			i, err := NumberToInt(arg)
			if err != nil {
				return nil, fmt.Errorf("%%%c format requires integer: %v", c, err)
			}
			switch c {
			case 'd', 'i':
				fmt.Fprintf(buf, "%d", i)
			case 'o':
				fmt.Fprintf(buf, "%o", i)
			case 'x':
				fmt.Fprintf(buf, "%x", i)
			case 'X':
				fmt.Fprintf(buf, "%X", i)
			}
		case 'e', 'f', 'g', 'E', 'F', 'G':
			f, ok := AsFloat(arg)
			if !ok {
				return nil, fmt.Errorf("%%%c format requires float, not %s", c, arg.Type())
			}
			Float(f).format(buf, c)
		case 'c':
			switch arg := arg.(type) {
			case Int:
				// chr(int)
				r, err := AsInt32(arg)
				if err != nil || r < 0 || r > unicode.MaxRune {
					return nil, fmt.Errorf("%%c format requires a valid Unicode code point, got %s", arg)
				}
				buf.WriteRune(rune(r))
			case String:
				r, size := utf8.DecodeRuneInString(string(arg))
				if size != len(arg) || len(arg) == 0 {
					return nil, fmt.Errorf("%%c format requires a single-character string")
				}
				buf.WriteRune(r)
			default:
				return nil, fmt.Errorf("%%c format requires int or single-character string, not %s", arg.Type())
			}
		case '%':
			buf.WriteByte('%')
		default:
			return nil, fmt.Errorf("unknown conversion %%%c", c)
		}
		format = format[1:]
		index++
	}

	if index < nargs {
		return nil, fmt.Errorf("too many arguments for format string")
	}

	return String(buf.String()), nil
}

type MaxAllocsError struct {
	Current, Max uint64
}

func (e *MaxAllocsError) Error() string {
	return "exceeded memory allocation limits"
}

// CheckAllocs returns an error if a change in allocations associated with this
// thread would be rejected by AddAllocs.
//
// It is safe to call CheckAllocs from any goroutine, even if the thread is
// actively executing.
func (thread *Thread) CheckAllocs(delta int64) error {
	thread.allocsLock.Lock()
	defer thread.allocsLock.Unlock()

	_, err := thread.simulateAllocs(delta)
	return err
}

// AddAllocs reports a change in allocations associated with this thread. If
// the total allocations exceed the limit defined via SetMaxAllocs, the thread
// is cancelled and an error is returned.
//
// It is safe to call AddAllocs from any goroutine, even if the thread is
// actively executing.
func (thread *Thread) AddAllocs(delta int64) error {
	thread.allocsLock.Lock()
	defer thread.allocsLock.Unlock()

	next, err := thread.simulateAllocs(delta)
	thread.allocs = next
	if err != nil {
		thread.Cancel(err.Error())
	}

	return err
}

// simulateAllocs simulates a call to AddAllocs returning the new total
// allocations associated with this thread and any error this would entail. No
// change is recorded.
func (thread *Thread) simulateAllocs(delta int64) (uint64, error) {
	if cancelReason := atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&thread.cancelReason))); cancelReason != nil {
		return thread.allocs, errors.New(*(*string)(cancelReason))
	}

	var nextAllocs uint64

	if delta < 0 {
		udelta := uint64(-delta)
		if udelta < thread.allocs {
			nextAllocs = thread.allocs - udelta
		} else {
			nextAllocs = 0
		}
		return nextAllocs, nil
	}

	udelta := uint64(delta)
	if udelta <= math.MaxUint64-thread.allocs {
		nextAllocs = thread.allocs + udelta
	} else {
		nextAllocs = math.MaxUint64
	}

	if vmdebug {
		fmt.Fprintf(os.Stderr, "allocation limit exceeded after %d steps: %d > %d", thread.steps, thread.allocs, thread.maxAllocs)
	}

	if thread.maxAllocs != 0 && nextAllocs > thread.maxAllocs {
		return nextAllocs, &MaxAllocsError{
			Current: thread.allocs,
			Max:     thread.maxAllocs,
		}
	}

	return nextAllocs, nil
}
