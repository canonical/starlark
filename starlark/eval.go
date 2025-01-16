// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"math/bits"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

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

	// contextLock synchronises access to fields required to implement context.
	contextLock   sync.Mutex
	parentContext context.Context
	cancelCleanup func()
	cancelReason  error
	done          chan struct{}

	// stack is the stack of (internal) call frames.
	stack []*frame

	// Print is the client-supplied implementation of the Starlark
	// 'print' function. If nil, fmt.Fprintln(os.Stderr, msg) is
	// used instead. This function must be completely safe as defined
	// by all existing safety flags requirements.
	Print func(thread *Thread, msg string)

	// Load is the client-supplied implementation of module loading.
	// Repeated calls with the same module name must return the same
	// module environment or error.
	// The error message need not include the module name.
	//
	// See example_test.go for some example implementations of Load.
	Load func(thread *Thread, module string) (StringDict, error)

	// Steps a count of abstract computation steps executed
	// by this thread. It is incremented by the interpreter. It may be used
	// as a measure of the approximate cost of Starlark execution, by
	// computing the difference in its value before and after a computation.
	//
	// The precise meaning of "step" is not specified and may change.
	steps     SafeInteger
	maxSteps  int64
	stepsLock sync.Mutex

	// allocs counts the abstract memory units claimed by this resource pool
	allocs     SafeInteger
	maxAllocs  int64
	allocsLock sync.Mutex

	// locals holds arbitrary "thread-local" Go values belonging to the client.
	// They are accessible to the client but not to any Starlark program.
	locals map[string]interface{}

	// proftime holds the accumulated execution time since the last profile event.
	proftime time.Duration

	// requiredSafety holds the set of safety conditions which must be
	// satisfied by any builtin which is called when running this thread.
	requiredSafety SafetyFlags
}

type threadContext Thread

var _ context.Context = &threadContext{}

func (tc *threadContext) Deadline() (deadline time.Time, ok bool) {
	thread := (*Thread)(tc)
	return thread.parentContext.Deadline()
}

var closedChannel chan struct{}

func init() {
	closedChannel = make(chan struct{})
	close(closedChannel)
}

func (tc *threadContext) Done() <-chan struct{} {
	thread := (*Thread)(tc)

	thread.contextLock.Lock()
	defer thread.contextLock.Unlock()

	if thread.done == nil {
		if thread.cancelReason == nil {
			thread.done = make(chan struct{})
		} else {
			// Don't set thread.done here, so we never risk closing it twice.
			return closedChannel
		}
	}
	return thread.done
}

func (tc *threadContext) Err() error {
	thread := (*Thread)(tc)

	thread.contextLock.Lock()
	defer thread.contextLock.Unlock()

	if thread.cancelReason != nil {
		if errors.Is(thread.cancelReason, context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return context.Canceled
	}
	return nil
}

func (tc *threadContext) Value(key interface{}) interface{} {
	thread := (*Thread)(tc)
	if stringKey, ok := key.(string); ok {
		if local, ok := thread.locals[stringKey]; ok {
			return local
		}
	}
	return tc.parentContext.Value(key)
}

// SetParentContext sets the parent for this thread's context. It
// can only be called once, before execution begins or any
// thread.Context calls.
func (thread *Thread) SetParentContext(ctx context.Context) {
	thread.contextLock.Lock()
	defer thread.contextLock.Unlock()

	if thread.parentContext != nil {
		panic("cannot set parent context: already set")
	}
	thread.parentContext = ctx

	stop := afterFunc(ctx, func() {
		thread.cancel(cause(ctx))
	})

	thread.cancelCleanup = func() { stop() }
}

// Context returns a context which gets cancelled when this thread is
// cancelled. Calling Value on the returned context with a string key is
// equivalent to calling thread.Local with that key.
//
// If Context is called, Cancel must also be called.
func (thread *Thread) Context() context.Context {
	thread.contextLock.Lock()
	defer thread.contextLock.Unlock()

	if thread.parentContext == nil {
		thread.parentContext = context.Background()
	}

	return (*threadContext)(thread)
}

// Steps returns the current value of Steps.
func (thread *Thread) Steps() (int64, bool) {
	thread.stepsLock.Lock()
	defer thread.stepsLock.Unlock()

	return thread.steps.Int64()
}

// SetMaxSteps sets a limit on the number of Starlark computation steps that
// may be executed by this thread. If the thread's step counter exceeds this
// limit, the thread is cancelled. If max is zero, negative or MaxInt64, the
// thread will not be cancelled.
func (thread *Thread) SetMaxSteps(max int64) {
	thread.maxSteps = max
}

// CheckSteps returns an error if an increase in steps taken
// by this thread would be rejected by AddSteps.
//
// It is safe to call CheckSteps from any goroutine, even if the thread
// is actively executing.
func (thread *Thread) CheckSteps(deltas ...SafeInteger) error {
	thread.stepsLock.Lock()
	defer thread.stepsLock.Unlock()

	_, err := thread.simulateSteps(deltas...)
	return err
}

// AddSteps reports an increase in the number of steps taken
// by this thread. If the new total steps exceeds the limit defined by
// SetMaxSteps, the thread is cancelled and an error is returned.
//
// It is safe to call AddSteps from any goroutine, even if the thread
// is actively executing.
func (thread *Thread) AddSteps(deltas ...SafeInteger) error {
	thread.stepsLock.Lock()
	defer thread.stepsLock.Unlock()

	deltas2 := make([]SafeInteger, len(deltas))
	for i, delta := range deltas {
		deltas2[i] = SafeInt(delta)
	}
	nextSteps, err := thread.simulateSteps(deltas2...)
	thread.steps = nextSteps
	if err != nil {
		thread.cancel(err)
	}

	return err
}

// simulateSteps simulates a call to AddSteps returning the
// new total step-count and any error this would entail. No change is
// recorded.
func (thread *Thread) simulateSteps(deltas ...SafeInteger) (SafeInteger, error) {
	if err := thread.cancelled(); err != nil {
		return thread.steps, err
	}

	nextSteps := thread.steps
	for _, delta := range deltas {
		nextSteps = SafeAdd(nextSteps, delta)

		nextSteps64, ok := nextSteps.Int64()
		if ok && thread.maxSteps > 0 && nextSteps64 > thread.maxSteps {
			return nextSteps, &StepsSafetyError{
				Current: thread.steps,
				Max:     thread.maxSteps,
			}
		}
	}
	if nextSteps64, ok := nextSteps.Int64(); ok && nextSteps64 < 0 {
		return SafeInteger{invalidSafeInt}, errors.New("step count invalidated")
	}
	return nextSteps, nil
}

// Allocs returns the total allocations reported to this thread via AddAllocs.
func (thread *Thread) Allocs() (int64, bool) {
	thread.allocsLock.Lock()
	defer thread.allocsLock.Unlock()

	return thread.allocs.Int64()
}

// SetMaxAllocs sets the maximum allocations that may be reported to this
// thread via AddAllocs before Cancel is internally called. If max is zero,
// negative or MaxInt64, the thread will not be cancelled.
func (thread *Thread) SetMaxAllocs(max int64) {
	thread.maxAllocs = max
}

// RequireSafety makes the thread only accept functions that declare at least
// the provided safety.
func (thread *Thread) RequireSafety(safety SafetyFlags) {
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
		return fmt.Errorf("thread safety: %w", err)
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
// Unlike most methods of Thread, it is safe to call Cancel from any
// goroutine, even if the thread is actively executing.
func (thread *Thread) Cancel(reason string, args ...interface{}) {
	var err error
	if len(args) == 0 {
		err = errors.New(reason)
	} else {
		err = fmt.Errorf(reason, args...)
	}
	thread.cancel(err)
}

func (thread *Thread) cancel(err error) {
	thread.contextLock.Lock()
	defer thread.contextLock.Unlock()

	if thread.cancelReason != nil {
		return
	}
	thread.cancelReason = fmt.Errorf("Starlark computation cancelled: %w", err)

	if thread.done != nil {
		close(thread.done)
	}

	if thread.cancelCleanup != nil {
		thread.cancelCleanup()
		thread.cancelCleanup = nil
	}
}

func (thread *Thread) cancelled() error {
	thread.contextLock.Lock()
	defer thread.contextLock.Unlock()

	return thread.cancelReason
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
// and which abides by safety limits. Errors prevent subsequent
// operations.
type SafeStringBuilder struct {
	builder       strings.Builder
	thread        *Thread
	allocs, steps int64
	err           error
}

var _ StringBuilder = &SafeStringBuilder{}

// NewSafeStringBuilder returns a StringBuilder which abides by
// the safety limits of this thread.
func NewSafeStringBuilder(thread *Thread) *SafeStringBuilder {
	return &SafeStringBuilder{thread: thread}
}

// Allocs returns the total allocations reported to this SafeStringBuilder's
// thread.
func (tb *SafeStringBuilder) Allocs() int64 {
	return tb.allocs
}

// Steps returns the total steps reported to this SafeStringBuilder's thread.
func (tb *SafeStringBuilder) Steps() int64 {
	return tb.steps
}

func (tb *SafeStringBuilder) safeGrow(n int) error {
	if tb.err != nil {
		return tb.err
	}

	if tb.Cap()-tb.Len() < n {
		// Make sure that we can allocate more
		newCap := tb.Cap()*2 + n
		newBufferSize := EstimateMakeSize([]byte{}, newCap)
		if tb.thread != nil {
			if err := tb.thread.AddAllocs(newBufferSize - int64(tb.allocs)); err != nil {
				tb.err = err
				return err
			}
		}
		// The real size of the allocated buffer might be
		// bigger than expected. For this reason, add the
		// difference between the real buffer size and the
		// target capacity, so that every allocated byte
		// is available to the user.
		tb.builder.Grow(n + int(newBufferSize) - newCap)
		tb.allocs = newBufferSize
	}
	return nil
}

func (tb *SafeStringBuilder) Grow(n int) {
	tb.safeGrow(n)
}

func (tb *SafeStringBuilder) Write(b []byte) (int, error) {
	if tb.thread != nil {
		if err := tb.thread.AddSteps(SafeInt(len(b))); err != nil {
			return 0, err
		}
	}
	if err := tb.safeGrow(len(b)); err != nil {
		return 0, err
	}

	return tb.builder.Write(b)
}

func (tb *SafeStringBuilder) WriteString(s string) (int, error) {
	if tb.thread != nil {
		if err := tb.thread.AddSteps(SafeInt(len(s))); err != nil {
			return 0, err
		}
	}
	if err := tb.safeGrow(len(s)); err != nil {
		return 0, err
	}

	return tb.builder.WriteString(s)
}

func (tb *SafeStringBuilder) WriteByte(b byte) error {
	if tb.thread != nil {
		if err := tb.thread.AddSteps(SafeInt(1)); err != nil {
			return err
		}
	}
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
	if tb.thread != nil {
		if err := tb.thread.CheckSteps(SafeInt(growAmount)); err != nil {
			return 0, err
		}
	}
	if err := tb.safeGrow(growAmount); err != nil {
		return 0, err
	}

	n, err := tb.builder.WriteRune(r)
	if err != nil {
		return 0, err
	}
	if tb.thread != nil {
		if err := tb.thread.AddSteps(SafeInt(n)); err != nil {
			return 0, err
		}
	}
	return n, nil
}

func (tb *SafeStringBuilder) Cap() int       { return tb.builder.Cap() }
func (tb *SafeStringBuilder) Len() int       { return tb.builder.Len() }
func (tb *SafeStringBuilder) String() string { return tb.builder.String() }
func (tb *SafeStringBuilder) Err() error     { return tb.err }

// A StringDict is a mapping from names to values, and represents
// an environment such as the global variables of a module.
// It is not a true starlark.Value.
type StringDict map[string]Value

var _ SafeStringer = StringDict(nil)

// Keys returns a new sorted slice of d's keys.
func (d StringDict) Keys() []string {
	names := make([]string, 0, len(d))
	for name := range d {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (d StringDict) SafeString(thread *Thread, sb StringBuilder) error {
	const safety = CPUSafe | MemSafe | TimeSafe | IOSafe
	if err := CheckSafety(thread, safety); err != nil {
		return err
	}
	if err := sb.WriteByte('{'); err != nil {
		return err
	}
	sep := ""
	for _, name := range d.Keys() {
		if _, err := sb.WriteString(sep); err != nil {
			return err
		}
		if _, err := sb.WriteString(name); err != nil {
			return err
		}
		if _, err := sb.WriteString(": "); err != nil {
			return err
		}
		if err := writeValue(thread, sb, d[name], nil); err != nil {
			return err
		}
		sep = ", "
	}
	if err := sb.WriteByte('}'); err != nil {
		return err
	}
	return nil
}

func (d StringDict) String() string {
	buf := new(strings.Builder)
	d.SafeString(nil, buf)
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

// ExecFile calls [ExecFileOptions] using [syntax.LegacyFileOptions].
//
// Deprecated: use [ExecFileOptions] with [syntax.FileOptions] instead,
// because this function relies on legacy global variables.
func ExecFile(thread *Thread, filename string, src interface{}, predeclared StringDict) (StringDict, error) {
	return ExecFileOptions(syntax.LegacyFileOptions(), thread, filename, src, predeclared)
}

// ExecFileOptions parses, resolves, and executes a Starlark file in the
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
// If ExecFileOptions fails during evaluation, it returns an *EvalError
// containing a backtrace.
func ExecFileOptions(opts *syntax.FileOptions, thread *Thread, filename string, src interface{}, predeclared StringDict) (StringDict, error) {
	// Parse, resolve, and compile a Starlark source file.
	_, mod, err := SourceProgramOptions(opts, filename, src, predeclared.Has)
	if err != nil {
		return nil, err
	}

	g, err := mod.Init(thread, predeclared)
	g.Freeze()
	return g, err
}

// SourceProgram calls [SourceProgramOptions] using [syntax.LegacyFileOptions].
//
// Deprecated: use [SourceProgramOptions] with [syntax.FileOptions] instead,
// because this function relies on legacy global variables.
func SourceProgram(filename string, src interface{}, isPredeclared func(string) bool) (*syntax.File, *Program, error) {
	return SourceProgramOptions(syntax.LegacyFileOptions(), filename, src, isPredeclared)
}

// SourceProgramOptions produces a new program by parsing, resolving,
// and compiling a Starlark source file.
// On success, it returns the parsed file and the compiled program.
// The filename and src parameters are as for syntax.Parse.
//
// The isPredeclared predicate reports whether a name is
// a pre-declared identifier of the current module.
// Its typical value is predeclared.Has,
// where predeclared is a StringDict of pre-declared values.
func SourceProgramOptions(opts *syntax.FileOptions, filename string, src interface{}, isPredeclared func(string) bool) (*syntax.File, *Program, error) {
	f, err := opts.Parse(filename, src, 0)
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
	compiled := compile.File(f.Options, f.Stmts, pos, "<toplevel>", module.Locals, module.Globals)

	return &Program{compiled}, nil
}

// CompiledProgram produces a new program from the representation
// of a compiled program previously saved by Program.Write.
func CompiledProgram(in io.Reader) (*Program, error) {
	data, err := io.ReadAll(in)
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
	compiled := compile.File(f.Options, f.Stmts, pos, "<toplevel>", module.Locals, module.Globals)
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

// Eval calls [EvalOptions] using [syntax.LegacyFileOptions].
//
// Deprecated: use [EvalOptions] with [syntax.FileOptions] instead,
// because this function relies on legacy global variables.
func Eval(thread *Thread, filename string, src interface{}, env StringDict) (Value, error) {
	return EvalOptions(syntax.LegacyFileOptions(), thread, filename, src, env)
}

// EvalOptions parses, resolves, and evaluates an expression within the
// specified (predeclared) environment.
//
// Evaluation cannot mutate the environment dictionary itself,
// though it may modify variables reachable from the dictionary.
//
// The filename and src parameters are as for syntax.Parse.
//
// If EvalOptions fails during evaluation, it returns an *EvalError
// containing a backtrace.
func EvalOptions(opts *syntax.FileOptions, thread *Thread, filename string, src interface{}, env StringDict) (Value, error) {
	expr, err := opts.ParseExpr(filename, src, 0)
	if err != nil {
		return nil, err
	}
	f, err := makeExprFunc(opts, expr, env)
	if err != nil {
		return nil, err
	}
	return Call(thread, f, nil, nil)
}

// EvalExpr calls [EvalExprOptions] using [syntax.LegacyFileOptions].
//
// Deprecated: use [EvalExprOptions] with [syntax.FileOptions] instead,
// because this function relies on legacy global variables.
func EvalExpr(thread *Thread, expr syntax.Expr, env StringDict) (Value, error) {
	return EvalExprOptions(syntax.LegacyFileOptions(), thread, expr, env)
}

// EvalExprOptions resolves and evaluates an expression within the
// specified (predeclared) environment.
// Evaluating a comma-separated list of expressions yields a tuple value.
//
// Resolving an expression mutates it.
// Do not call EvalExprOptions more than once for the same expression.
//
// Evaluation cannot mutate the environment dictionary itself,
// though it may modify variables reachable from the dictionary.
//
// If EvalExprOptions fails during evaluation, it returns an *EvalError
// containing a backtrace.
func EvalExprOptions(opts *syntax.FileOptions, thread *Thread, expr syntax.Expr, env StringDict) (Value, error) {
	fn, err := makeExprFunc(opts, expr, env)
	if err != nil {
		return nil, err
	}
	return Call(thread, fn, nil, nil)
}

// ExprFunc calls [ExprFuncOptions] using [syntax.LegacyFileOptions].
//
// Deprecated: use [ExprFuncOptions] with [syntax.FileOptions] instead,
// because this function relies on legacy global variables.
func ExprFunc(filename string, src interface{}, env StringDict) (*Function, error) {
	return ExprFuncOptions(syntax.LegacyFileOptions(), filename, src, env)
}

// ExprFunc returns a no-argument function
// that evaluates the expression whose source is src.
func ExprFuncOptions(options *syntax.FileOptions, filename string, src interface{}, env StringDict) (*Function, error) {
	expr, err := options.ParseExpr(filename, src, 0)
	if err != nil {
		return nil, err
	}
	return makeExprFunc(options, expr, env)
}

// makeExprFunc returns a no-argument function whose body is expr.
// The options must be consistent with those used when parsing expr.
func makeExprFunc(opts *syntax.FileOptions, expr syntax.Expr, env StringDict) (*Function, error) {
	locals, err := resolve.ExprOptions(opts, expr, env.Has, Universe.Has)
	if err != nil {
		return nil, err
	}

	return makeToplevelFunction(compile.Expr(opts, expr, "<expr>", locals), env), nil
}

// The following functions are primitive operations of the byte code interpreter.

// list += iterable
func safeListExtend(thread *Thread, x *List, y Iterable) error {
	elemsAppender := NewSafeAppender(thread, &x.elems)
	if ylist, ok := y.(*List); ok {
		// fast path: list += list

		// Equalise step cost for fast and slow path.
		if err := thread.AddSteps(SafeInt(len(ylist.elems))); err != nil {
			return err
		}
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
func getAttr(thread *Thread, x Value, name string, hint bool) (Value, error) {
	if x, ok := x.(HasAttrs); ok {
		var attr Value
		var err error
		if x2, ok := x.(HasSafeAttrs); ok {
			attr, err = x2.SafeAttr(thread, name)
		} else {
			if err := CheckSafety(thread, NotSafe); err != nil {
				return nil, err
			}
			attr, err = x.Attr(name)
			if attr == nil && err == nil {
				err = ErrNoAttr
			}
		}

		if err != nil {
			var errmsg string
			if nsa, ok := err.(NoSuchAttrError); ok {
				errmsg = string(nsa)
			} else if err == ErrNoAttr {
				errmsg = fmt.Sprintf("%s has no .%s field or method", x.Type(), name)
			} else {
				return nil, err // return error as is
			}

			// add spelling hint
			if hint {
				if n := spell.Nearest(name, x.AttrNames()); n != "" {
					errmsg = fmt.Sprintf("%s (did you mean .%s?)", errmsg, n)
				}
			}

			return nil, errors.New(errmsg)
		}
		return attr, nil
	}
	return nil, fmt.Errorf("%s has no .%s field or method", x.Type(), name)
}

// setField implements x.name = y.
func setField(thread *Thread, x Value, name string, y Value) error {
	if x, ok := x.(HasSetField); ok {
		var err error
		if x2, ok := x.(HasSafeSetField); ok {
			err = x2.SafeSetField(thread, name, y)
		} else if err = CheckSafety(thread, NotSafe); err == nil {
			err = x.SetField(name, y)
		}

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
func getIndex(thread *Thread, x, y Value) (Value, error) {
	switch x := x.(type) {
	case Mapping: // dict
		var z Value
		var found bool
		var err error
		if x2, ok := x.(SafeMapping); ok {
			z, found, err = x2.SafeGet(thread, y)
		} else if err := CheckSafety(thread, NotSafe); err != nil {
			return nil, err
		} else {
			z, found, err = x.Get(y)
		}
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
		if x, ok := x.(SafeIndexable); ok {
			return x.SafeIndex(thread, i)
		}
		if err := CheckSafety(thread, NotSafe); err != nil {
			return nil, err
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
	return SafeUnary(nil, op, x)
}

// SafeUnary applies a unary operator (+, -, ~, not) to its operand,
// respecting safety.
func SafeUnary(thread *Thread, op syntax.Token, x Value) (Value, error) {
	// The NOT operator is not customizable.
	if op == syntax.NOT {
		return !x.Truth(), nil
	}

	if x, ok := x.(HasSafeUnary); ok {
		return x.SafeUnary(thread, op)
	}

	// Int, Float, and user-defined types
	if x, ok := x.(HasUnary); ok {
		if err := CheckSafety(thread, NotSafe); err != nil {
			return nil, err
		}
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
	const safety = CPUSafe | MemSafe | TimeSafe | IOSafe
	if err := CheckSafety(thread, safety); err != nil {
		return nil, err
	}

	intLenSteps := func(i Int) SafeInteger {
		if _, iBig := i.get(); iBig != nil {
			return SafeDiv(iBig.BitLen(), 32)
		}
		return SafeInt(0)
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
					resultLen := len(x) + len(y)
					if err := thread.AddSteps(SafeInt(resultLen)); err != nil {
						return nil, err
					}
					resultSize := EstimateMakeSize([]byte{}, resultLen) + StringTypeOverhead
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
					if err := thread.AddSteps(safeMax(intLenSteps(x), intLenSteps(y))); err != nil {
						return nil, err
					}
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
				resultLen := SafeAdd(x.Len(), y.Len())
				resultLen64, ok := resultLen.Int64()
				if !ok {
					return nil, errors.New("result len overflowed")
				}

				if thread != nil {
					if err := thread.AddSteps(resultLen); err != nil {
						return nil, err
					}
					resultSize := EstimateMakeSize([]Value{}, x.Len()+y.Len()) + EstimateSize(&List{})
					if err := thread.AddAllocs(resultSize); err != nil {
						return nil, err
					}
				}

				z := make([]Value, 0, resultLen64)
				z = append(z, x.elems...)
				z = append(z, y.elems...)
				return NewList(z), nil
			}
		case Tuple:
			if y, ok := y.(Tuple); ok {
				resultLen := SafeAdd(len(x), len(y))
				resultLen64, ok := resultLen.Int64()
				if !ok {
					return nil, errors.New("result len overflowed")
				}

				if thread != nil {
					if err := thread.AddSteps(resultLen); err != nil {
						return nil, err
					}
					zSize := EstimateMakeSize(Tuple{}, len(x)+len(y)) + SliceTypeOverhead
					if err := thread.AddAllocs(zSize); err != nil {
						return nil, err
					}
				}

				z := make(Tuple, 0, resultLen64)
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
				if thread != nil {
					if err := thread.AddSteps(safeMax(intLenSteps(x), intLenSteps(y))); err != nil {
						return nil, err
					}
					if err := thread.CheckAllocs(max(EstimateSize(x), EstimateSize(y))); err != nil {
						return nil, err
					}
					result := Value(x.Sub(y))
					if err := thread.AddAllocs(EstimateSize(result)); err != nil {
						return nil, err
					}
					return result, nil
				}
				return x.Sub(y), nil
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
				return xf - y, nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return x - y, nil
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
				return x - yf, nil
			}
		case *Set: // difference
			if y, ok := y.(*Set); ok {
				iter, err := SafeIterate(thread, y)
				if err != nil {
					return nil, err
				}
				defer iter.Done()
				diff, err := x.safeDifference(thread, iter)
				if err != nil {
					return nil, err
				}
				if err := iter.Err(); err != nil {
					return nil, err
				}
				return diff, nil
			}
		}

	case syntax.STAR:
		switch x := x.(type) {
		case Int:
			switch y := y.(type) {
			case Int:
				if thread != nil {
					// In the worse case, Karatsuba's algorithm is used.
					lenSteps64, ok := safeMax(intLenSteps(x), intLenSteps(y)).Int64()
					if !ok {
						return nil, errors.New("result len overflowed")
					}
					resultSteps := SafeInt(math.Pow(float64(lenSteps64), 1.58))
					if err := thread.AddSteps(resultSteps); err != nil {
						return nil, err
					}
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
					if err := thread.AddAllocs(floatSize); err != nil {
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
					if err := thread.AddAllocs(floatSize); err != nil {
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
					if err := thread.AddAllocs(floatSize); err != nil {
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
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return xf / yf, nil
			case Float:
				if y == 0.0 {
					return nil, fmt.Errorf("floating-point division by zero")
				}
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return xf / y, nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if y == 0.0 {
					return nil, fmt.Errorf("floating-point division by zero")
				}
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
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
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
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
				if thread != nil {
					// Integer division is hard - most implementations are O(n^2).
					// Although implementations exist which turn division into
					// multiplication, making this cost same as `STAR` operator,
					// Go does not yet do this.
					resultSteps := safeMax(intLenSteps(x), intLenSteps(y))
					resultSteps = SafeMul(resultSteps, resultSteps)
					if err := thread.AddSteps(resultSteps); err != nil {
						return nil, err
					}
					if resultSizeEstimate := EstimateSize(x) - EstimateSize(y); resultSizeEstimate > 0 {
						if err := thread.CheckAllocs(resultSizeEstimate); err != nil {
							return nil, err
						}
					}
					result := Value(x.Div(y))
					if err := thread.AddAllocs(EstimateSize(result)); err != nil {
						return nil, err
					}
					return result, nil
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
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return floor(xf / y), nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if y == 0.0 {
					return nil, fmt.Errorf("floored division by zero")
				}
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
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
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
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
				if thread != nil {
					// Modulo is the same as division in terms of complexity.
					// Integer division is hard - most implementations are O(n^2).
					// Although implementations exist which turn division into
					// multiplication, making this cost same as `STAR` operator,
					// Go does not yet do this.
					resultSteps := safeMax(intLenSteps(x), intLenSteps(y))
					resultSteps = SafeMul(resultSteps, resultSteps)
					if err := thread.AddSteps(resultSteps); err != nil {
						return nil, err
					}
					if err := thread.CheckAllocs(EstimateSize(y)); err != nil {
						return nil, err
					}
				}
				result := Value(x.Mod(y))
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(result)); err != nil {
						return nil, err
					}
				}
				return result, nil
			case Float:
				xf, err := x.finiteFloat()
				if err != nil {
					return nil, err
				}
				if y == 0 {
					return nil, fmt.Errorf("floating-point modulo by zero")
				}
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return xf.Mod(y), nil
			}
		case Float:
			switch y := y.(type) {
			case Float:
				if y == 0.0 {
					return nil, fmt.Errorf("floating-point modulo by zero")
				}
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
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
				if thread != nil {
					if err := thread.AddAllocs(floatSize); err != nil {
						return nil, err
					}
				}
				return x.Mod(yf), nil
			}
		case String:
			return interpolate(thread, string(x), y)
		}

	case syntax.NOT_IN:
		z, err := safeBinary(thread, syntax.IN, x, y)
		if err != nil {
			return nil, err
		}
		return !z.Truth(), nil

	case syntax.IN:
		switch y := y.(type) {
		case *List:
			for _, elem := range y.elems {
				if thread != nil {
					if err := thread.AddSteps(SafeInt(1)); err != nil {
						return nil, err
					}
				}
				if eq, err := Equal(elem, x); err != nil {
					return nil, err
				} else if eq {
					return True, nil
				}
			}
			return False, nil
		case Tuple:
			for _, elem := range y {
				if thread != nil {
					if err := thread.AddSteps(SafeInt(1)); err != nil {
						return nil, err
					}
				}
				if eq, err := Equal(elem, x); err != nil {
					return nil, err
				} else if eq {
					return True, nil
				}
			}
			return False, nil
		case Mapping: // e.g. dict
			if y, ok := y.(SafeMapping); ok {
				_, found, err := y.SafeGet(thread, x)
				if errors.Is(err, ErrSafety) {
					return nil, err
				}
				return Bool(found), nil
			}

			if err := CheckSafety(thread, NotSafe); err != nil {
				return nil, err
			}
			// Ignore error from Get as we cannot distinguish true
			// errors (value cycle, type error) from "key not found".
			_, found, _ := y.Get(x)
			return Bool(found), nil
		case *Set:
			ok, err := y.safeHas(thread, x)
			if err != nil {
				return nil, err
			}
			return Bool(ok), nil
		case String:
			needle, ok := x.(String)
			if !ok {
				return nil, fmt.Errorf("'in <string>' requires string as left operand, not %s", x.Type())
			}
			if thread != nil {
				if err := thread.AddSteps(SafeInt(len(y))); err != nil {
					return nil, err
				}
			}
			return Bool(strings.Contains(string(y), string(needle))), nil
		case Bytes:
			switch needle := x.(type) {
			case Bytes:
				if thread != nil {
					if err := thread.AddSteps(SafeInt(len(y))); err != nil {
						return nil, err
					}
				}
				return Bool(strings.Contains(string(y), string(needle))), nil
			case Int:
				var b byte
				if err := AsInt(needle, &b); err != nil {
					return nil, fmt.Errorf("int in bytes: %s", err)
				}
				if thread != nil {
					if err := thread.AddSteps(SafeInt(len(y))); err != nil {
						return nil, err
					}
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
				if thread != nil {
					if err := thread.AddSteps(safeMax(intLenSteps(x), intLenSteps(y))); err != nil {
						return nil, err
					}
					if err := thread.CheckAllocs(max(EstimateSize(x), EstimateSize(y))); err != nil {
						return nil, err
					}
				}
				result := Value(x.Or(y))
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(result)); err != nil {
						return nil, err
					}
				}
				return result, nil
			}

		case *Dict: // union
			if y, ok := y.(*Dict); ok {
				return x.safeUnion(thread, y)
			}

		case *Set: // union
			if y, ok := y.(*Set); ok {
				iter, err := SafeIterate(thread, y)
				if err != nil {
					return nil, err
				}
				defer iter.Done()
				z, err := x.safeUnion(thread, iter)
				if err != nil {
					return nil, err
				}
				if err := iter.Err(); err != nil {
					return nil, err
				}
				return z, nil
			}
		}

	case syntax.AMP:
		switch x := x.(type) {
		case Int:
			if y, ok := y.(Int); ok {
				if thread != nil {
					if err := thread.AddSteps(safeMax(intLenSteps(x), intLenSteps(y))); err != nil {
						return nil, err
					}
					resultSize := max(EstimateSize(x), EstimateSize(y))
					if err := thread.AddAllocs(resultSize); err != nil {
						return nil, err
					}
				}
				return x.And(y), nil
			}
		case *Set: // intersection
			if y, ok := y.(*Set); ok {
				iter, err := SafeIterate(thread, y)
				if err != nil {
					return nil, err
				}
				defer iter.Done()
				z, err := x.safeIntersection(thread, iter)
				if err != nil {
					return nil, err
				}
				if err := iter.Err(); err != nil {
					return nil, err
				}
				return z, err
			}
		}

	case syntax.CIRCUMFLEX:
		switch x := x.(type) {
		case Int:
			if y, ok := y.(Int); ok {
				if thread != nil {
					if err := thread.AddSteps(safeMax(intLenSteps(x), intLenSteps(y))); err != nil {
						return nil, err
					}
					resultSize := max(EstimateSize(x), EstimateSize(y))
					if err := thread.AddAllocs(resultSize); err != nil {
						return nil, err
					}
				}
				return x.Xor(y), nil
			}
		case *Set: // symmetric difference
			if y, ok := y.(*Set); ok {
				iter, err := SafeIterate(thread, y)
				if err != nil {
					return nil, err
				}
				defer iter.Done()
				z, err := x.safeSymmetricDifference(thread, iter)
				if err != nil {
					return nil, err
				}
				if err := iter.Err(); err != nil {
					return nil, err
				}
				return z, nil
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
				if thread != nil {
					if err := thread.AddSteps(SafeAdd(intLenSteps(x), SafeDiv(y, 32))); err != nil {
						return nil, err
					}
					if err := thread.CheckAllocs(EstimateSize(x)); err != nil {
						return nil, err
					}
				}
				z := x.Lsh(uint(y))
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(z)); err != nil {
						return nil, err
					}
				}
				return z, nil
			} else {
				if thread != nil {
					if err := thread.AddSteps(safeMax(SafeSub(intLenSteps(x), SafeDiv(y, 32)), SafeInt(0))); err != nil {
						return nil, err
					}
					if err := thread.CheckAllocs(EstimateSize(x)); err != nil {
						return nil, err
					}
				}
				z := x.Rsh(uint(y))
				if thread != nil {
					if err := thread.AddAllocs(EstimateSize(z)); err != nil {
						return nil, err
					}
				}
				return z, nil
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
	of, sz := bits.Mul(uint(len(elems)), uint(i))
	if of != 0 || sz >= maxAlloc { // of != 0 => overflow
		// Don't print sz.
		return nil, fmt.Errorf("excessive repeat (%d * %d elements)", len(elems), i)
	}
	if thread != nil {
		if err := thread.AddSteps(SafeInt(sz)); err != nil {
			return nil, err
		}
		if err := thread.AddAllocs(EstimateMakeSize([]Value{}, int(sz))); err != nil {
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
	of, sz := bits.Mul(uint(len(s)), uint(i))
	if of != 0 || sz >= maxAlloc { // of != 0 => overflow
		// Don't print sz.
		return "", fmt.Errorf("excessive repeat (%d * %d elements)", len(s), i)
	}
	if thread != nil {
		if err := thread.AddSteps(SafeInt(sz)); err != nil {
			return "", err
		}
		if err := thread.AddAllocs(EstimateMakeSize([]byte{}, int(sz))); err != nil {
			return "", err
		}
	}
	return String(strings.Repeat(string(s), i)), nil
}

// Max depth of a Starlark stack. This is significantly less than Go's own hard
// limit and greater than startest's maximum st.N.
const maxStackDepth = 110_000

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
			return nil, fmt.Errorf("cannot call builtin '%s': %w", b.Name(), err)
		}
		return nil, fmt.Errorf("cannot call value of type '%s': %w", c.Type(), err)
	}

	if len(thread.stack)+1 >= maxStackDepth {
		return nil, fmt.Errorf("stack overflow")
	}

	// Allocate and push a new frame.
	var fr *frame
	// Optimization: use slack portion of thread.stack
	// slice as a freelist of empty frames.
	if n := len(thread.stack); n < cap(thread.stack) {
		fr = thread.stack[n : n+1][0]
	}
	if fr == nil {
		if err := thread.AddAllocs(EstimateSize(&frame{})); err != nil {
			return nil, err
		}
		fr = new(frame)
	}

	// one-time initialization of thread
	if thread.maxSteps == 0 {
		thread.maxSteps-- // (MaxUint64)
	}

	// Count only for stack memory as other resources are already
	// accounted for.
	prevStackCap := cap(thread.stack)
	thread.stack = append(thread.stack, fr)
	if newStackCap := cap(thread.stack); prevStackCap != newStackCap {
		prevStackSize := EstimateMakeSize([]*frame{}, prevStackCap)
		newStackSize := EstimateMakeSize([]*frame{}, newStackCap)
		delta := newStackSize - prevStackSize
		if err := thread.AddAllocs(delta); err != nil {
			return nil, err
		}
	}

	fr.callable = c

	thread.beginProfSpan()

	// Use defer to ensure that panics from built-ins
	// pass through the interpreter without leaving
	// it in a bad state.
	defer func() {
		thread.endProfSpan()

		// clear out any references
		// TODO(adonovan): opt: zero fr.Locals and
		// reuse it if it is large enough.
		*fr = frame{}

		thread.stack = thread.stack[:len(thread.stack)-1] // pop
	}()

	result, err := c.CallInternal(thread, args, kwargs)

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
func interpolate(thread *Thread, format string, x Value) (Value, error) {
	buf := NewSafeStringBuilder(thread)
	index := 0
	nargs := 1
	if tuple, ok := x.(Tuple); ok {
		nargs = len(tuple)
	}
	for {
		i := strings.IndexByte(format, '%')
		if i < 0 {
			if _, err := buf.WriteString(format); err != nil {
				return nil, err
			}
			break
		}
		if _, err := buf.WriteString(format[:i]); err != nil {
			return nil, err
		}
		format = format[i+1:]

		if format != "" && format[0] == '%' {
			if err := buf.WriteByte('%'); err != nil {
				return nil, err
			}
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
			var v Value
			var found bool
			switch x := x.(type) {
			case SafeMapping:
				var err error
				v, found, err = x.SafeGet(thread, String(key))
				if errors.Is(err, ErrSafety) {
					return nil, err
				}
			case Mapping:
				if err := CheckSafety(thread, NotSafe); err != nil {
					return nil, err
				}
				v, found, _ = x.Get(String(key))
			default:
				return nil, fmt.Errorf("format requires a mapping")
			}
			if !found {
				return nil, fmt.Errorf("key not found: %s", key)
			}
			arg = v
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
				if _, err := buf.WriteString(str); err != nil {
					return nil, err
				}
			} else {
				if err := writeValue(thread, buf, arg, nil); err != nil {
					return nil, err
				}
			}
		case 'd', 'i', 'o', 'x', 'X':
			i, err := NumberToInt(arg)
			if err != nil {
				return nil, fmt.Errorf("%%%c format requires integer: %v", c, err)
			}
			switch c {
			case 'd', 'i':
				if _, err := fmt.Fprintf(buf, "%d", i); err != nil {
					return nil, err
				}
			case 'o':
				if _, err := fmt.Fprintf(buf, "%o", i); err != nil {
					return nil, err
				}
			case 'x':
				if _, err := fmt.Fprintf(buf, "%x", i); err != nil {
					return nil, err
				}
			case 'X':
				if _, err := fmt.Fprintf(buf, "%X", i); err != nil {
					return nil, err
				}
			}
		case 'e', 'f', 'g', 'E', 'F', 'G':
			f, ok := AsFloat(arg)
			if !ok {
				return nil, fmt.Errorf("%%%c format requires float, not %s", c, arg.Type())
			}
			if err := Float(f).format(buf, c); err != nil {
				return nil, err
			}
		case 'c':
			switch arg := arg.(type) {
			case Int:
				// chr(int)
				r, err := AsInt32(arg)
				if err != nil || r < 0 || r > unicode.MaxRune {
					return nil, fmt.Errorf("%%c format requires a valid Unicode code point, got %s", arg)
				}
				if _, err := buf.WriteRune(rune(r)); err != nil {
					return nil, err
				}
			case String:
				r, size := utf8.DecodeRuneInString(string(arg))
				if size != len(arg) || len(arg) == 0 {
					return nil, fmt.Errorf("%%c format requires a single-character string")
				}
				if _, err := buf.WriteRune(r); err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("%%c format requires int or single-character string, not %s", arg.Type())
			}
		case '%':
			if err := buf.WriteByte('%'); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unknown conversion %%%c", c)
		}
		format = format[1:]
		index++
	}
	if err := buf.Err(); err != nil {
		return nil, err
	}

	if index < nargs {
		if _, ok := x.(Mapping); !ok {
			return nil, fmt.Errorf("too many arguments for format string")
		}
	}

	if thread != nil {
		if err := thread.AddAllocs(StringTypeOverhead); err != nil {
			return nil, err
		}
	}
	return String(buf.String()), nil
}

type AllocsSafetyError struct {
	Current SafeInteger
	Max     int64
}

func (e *AllocsSafetyError) Error() string {
	return "exceeded memory allocation limits"
}

func (e *AllocsSafetyError) Is(err error) bool {
	return err == ErrSafety
}

type StepsSafetyError struct {
	Current SafeInteger
	Max     int64
}

func (e *StepsSafetyError) Error() string {
	return "too many steps"
}

func (e *StepsSafetyError) Is(err error) bool {
	return err == ErrSafety
}

// CheckAllocs returns an error if a change in allocations associated with this
// thread would be rejected by AddAllocs.
//
// It is safe to call CheckAllocs from any goroutine, even if the thread is
// actively executing.
func (thread *Thread) CheckAllocs(deltas ...int64) error {
	thread.allocsLock.Lock()
	defer thread.allocsLock.Unlock()

	_, err := thread.simulateAllocs(deltas...)
	return err
}

// AddAllocs reports a change in allocations associated with this thread. If
// the total allocations exceed the limit defined via SetMaxAllocs, the thread
// is cancelled and an error is returned.
//
// It is safe to call AddAllocs from any goroutine, even if the thread is
// actively executing.
func (thread *Thread) AddAllocs(deltas ...int64) error {
	thread.allocsLock.Lock()
	defer thread.allocsLock.Unlock()

	next, err := thread.simulateAllocs(deltas...)
	thread.allocs = next
	if err != nil {
		thread.cancel(err)
	}

	return err
}

// simulateAllocs simulates a call to AddAllocs returning the new total
// allocations associated with this thread and any error this would entail. No
// change is recorded.
func (thread *Thread) simulateAllocs(deltas ...int64) (SafeInteger, error) {
	nextAllocs := thread.allocs
	for _, delta := range deltas {
		nextAllocs = SafeAdd(nextAllocs, delta)

		nextAllocs64, ok := nextAllocs.Int64()
		if ok && thread.maxAllocs > 0 && nextAllocs64 > thread.maxAllocs {
			return nextAllocs, &AllocsSafetyError{
				Current: thread.allocs,
				Max:     thread.maxAllocs,
			}
		}
	}
	if nextAllocs64, ok := nextAllocs.Int64(); ok && nextAllocs64 < 0 {
		return SafeInteger{invalidSafeInt}, errors.New("alloc count invalidated")
	}
	return nextAllocs, nil
}
