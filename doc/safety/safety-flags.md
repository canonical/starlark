---
tags: starlark, safety, safety flags
---

# Safety Flags

Starlark script execution has two main parts: instructions and builtins.

In fact, the script needs to first be *compiled* to a stack-oriented instruction set which targets a very simple *virtual processor*. This enable nice performance and makes it possible to run the same script multiple times without inspecting and validating the syntax at every run. Moreover, the virtual processor can only operate in RAM: it can never access OS features. This makes Starlark's execution sandboxed by default.

It is clear, however, that most useful systems cannot rely on computation alone, but need some (preferably limited) access to the external world. *Builtins* are the "glue" necessary to do that. A builtin is just a plain Go function which can do anything unconstrained[^1]. This calls for a problem in safety. A user might be interested in constraining memory, but might not care about I/O or CPU. So, there should be a way to disable all *builtins* which cannot cooperatively manage memory, for example. This is what *SafetyFlags* are for: they label parts of the computation as safe or not safe in respect to on one or more aspecs of safety.

There are four safety flags, for now:

 - `MemSafe` for memory,
 - `CPUSafe` for CPU usage,
 - `TimeSafe` for time (cancellation tokens),
 - `IOSafe` for IO.

## Limiting execution

The object that contains all safety-related counters and flags is the `Thread`. Upon instantiation, or in general before execution, it is possible to limit a thread to run a subset of the language using the `RequireSafety(SafetyFlag)` method like:

```go
thread := &starlark.Thread{}

// SafetyFlags are append-only.
thread.RequireSafety(starlark.CPUSafe)
thread.RequireSafety(starlark.MemSafe)

// They can be specified together by or-ing them together like
thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)

// Multiple calls with the same flags, while redundant, are ok
thread.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
thread.RequireSafety(starlark.CPUSafe | starlark.IOSafe)
// The thread will require MemSafe, CPUSafe and IOSafe

// If the line becomes too big, it is good practice to use a 
// constant which should be called `safety`, like:
const safety = CPUSafe | MemSafe | IOSafe
thread.RequireSafety(safety)
// The order of the flags, by convention, should follow the 
// order in the declaration: CPU, Mem, Time and last IO.
```

As an important note, limiting to CPUSafe only does not mean that resource management will not be enforced (like memory or time), but just that functions that are not capable of enforcing that limit can run.

## Programming interface

There are mainly two facilities that can be used to mark a function safe from a `SafetyFlag`s pov: `Thread.CheckPermits(SafetyAware)` (method) and `CheckSafety(*Thread, any)` (free standing function). Both of these functions check if a thread allows execution af a function which is marked with the flags passed and return an error if they are not. The main difference between them is that the former (the `Thread`) to be not-nil, while the latter accepts a nil thread. In case `thread` is nil, *all execution is allowed*. This means that, from the point of view safety, not having a thread is equivalent to having a thread marked as `NotSafe`. The rationale behind this is that if a routine does not have access to a thread, it means that it might being used outside of a starlark's script execution. As such, it's not this library's duty to limit its execution[^2].

It can be observed that both functions take as a parameter a `SafetyAware` (in the case of `CheckSafety` it's actually `any`, as a means to be more flexible, however it will be unpacked, if possible, into a `SafetyAware` within the function) and not a plain `SafetyFlag`. `SafetyAware` is a very simple interface whith a single method:

```go
type SafetyAware interface {
	Safety() SafetyFlags
}
```

`SafetyFlag`s themselves implement `SafetyAware`.

The reason for this indirection is that some objects inherently represent a *single* run-time operation and, as such, they can be uniquely bound to a defined execution safety. They usually have few other properties like being transient, being bound to a single execution thread and being usually used only by the internals of Starlark's interpreter. Clear examples of this are `Builtin`s and `Iterator`s. This pattern also helps porting existing libraries to "safe Starlark".

In the rest of the cases, it is necessary to add a `SafeXXX` version of the routine, taking a `*Thread` as the first parameter[^3]. This include many of the existing interfaces used by Starlark's interpreter of which a safe version has been added, for example:

```go

// fmt.Stringer's safe version
type SafeStringer interface {
	SafeString(thread *Thread, sb StringBuilder) error
}

type SafeIndexable interface {
	Indexable
	SafeIndex(thread *Thread, i int) (Value, error)
}
...
```

All the `SafeXXX` are expected to handle a `nil` thread as a valid argument meaning "no safety is required". In fact, as a convention, it is expected that `XXX(...)` is equivalent to it's safe version when an nil thread is passed `SafeXXX(nil, ...)`[^4]. Each `SafeXXX` method is also required to coopratively check for `SafetyFlag`s, usually at the beginning of the function; `starlark.CheckSafety` helps reduce the boilerplate to something on the lines of:

```go
func (v *MyValue) SafeXXX(thread *starlark.Thread, ...) error {
    const safety = ...;
    if err := starlark.CheckSafety(safety); err != nil {
        return err
    }
    ...
}
```

### Builtins

Builtins can define their safety in two ways: by using  `starlark.NewBuiltinWithSafety` like:

```go
var Module = &starlarkstruct.Module{
    Name: "awesomeModule",
    Members: starlark.StringDict{
        "awesome": starlark.NewBuiltinWithSafety("awesome", starlark.MemSafe, awesome),
        ...
    },
}
```

or by calling `builtin.DeclareSafety` after construction like:

```go
var Module = &starlarkstruct.Module{
    Name: "awesomeModule",
    Members: starlark.StringDict{
        "awesome": starlark.NewBuiltin("awesome", awesome),
        ...
    },
}

var safeties = map[string]starlark.SafetyFlags{
    "awesome": starlark.MemSafe,
    ...
}

func init() {
    for name, safety := range safeties {
        if v, ok := Module.Members[name]; ok {
            if builtin, ok := v.(*starlark.Builtin); ok {
                builtin.DeclareSafety(safety)
            }
        }
    }
}
```

While the second method seems complex and possibly slower, it is the prefered solution when porting an existing library as it reduces the chances of conflict with an upstream since it does not modify the original declaration. Moreover, in this case, a test can be added to make sure that, if the upstream adds a new function, it must be addressed by downstream. An example of such test is:

```go
func TestModuleSafeties(t *testing.T) {
    for name, value := range awesome.Module.Members {
        builtin, ok := value.(*starlark.Builtin)
        if !ok {
            continue
        }

        // Clearly, in this test `Safeties` should be exported. This
        // can be achieved by adding an export_test.go whith the same
        // package under the module directory which just exports that
        // variable. This way only the test package can access it.
        if safety, ok := (*awesome.Safeties)[name]; !ok {
            t.Errorf("builtin awesome.%s has no safety declaration", name)
        } else if actual := builtin.Safety(); actual != safety {
            t.Errorf("builtin awesome.%s has incorrect safety: expected %v but got %v", name, safety, actual)
        }
    }

    for name, _ := range *awesome.Safeties {
        if _, ok := awesome.Module.Members[name]; !ok {
            t.Errorf("safety declared for non-existent builtin awesome.%s", name)
        }
    }
}
```

In all other cases, using `starlark.NewBuiltinWithSafety` should be prefered.

### Safe interfaces

Most of the interfaces used by the interpreter have been given a `SafeXXX` version. One notable exception is `HasBinary` which is right now unsafe-only as it would require a lot of work to make it safe with very little gain. It may be added in future work. All *unsafe* interfaces still work if the thread is marked as `NotSafe`, making this addition mostly backward-compatible.

The complete list of safe-aware interfaces is:

 - `SafeStringer` (safe `fmt.Stringer`)
 - `SafeIndexable` (safe `Indexable`)
 - `HasSafeSetIndex` (safe `HasSetIndex`)
 - `SafeIterator` (safe `Iterator`)
 - `SafeMapping` (safe `Mapping`)
 - `HasSafeSetKey` (safe `HasSetKey`)
 - `HasSafeUnary` (safe `HasUnary`)
 - `HasSafeAttrs` (safe `HasAttrs`)

A part from the additional `*Thread` argument and sometimes the `error` return value, these interfaces are very similar to the unsafe version in behavior. The only exceptions are [`SafeIterator`s](#Iterators) and `HasSafeAttrs`. The latter, has a different semantics for return values:
 
 - the original interface can `return nil, nil` to signal a missing attribute;
 - the safe one follows standard Go practice and can either return a `Value` or an `error`. If the attribute doesn't exists, it can `return nil, starlark.ErrNoSuchAttr` instead of `nil, nil`.

### Iterators

Iterators are objects returned by the `Iterable.Iterate` method. They are short lived, use-once objects used both by the interpreter and by other methods. Sometimes iterating over an object is the only way to access the elements of a container (for example, `Dict` and `Set`). Their ephemeral nature makes it possible to add safety *to the type* instead. The pattern used to make them safe aims to support both unsafe and safe usage.

However, this library adds a function to the `Iterator` interface: `Err() error`. This is one of the very few breaking changes from upstream. The general feeling, however, is that it was a mistake to model iteration as a never failing execution. The usual usage pattern for `Iterator` can thus be improved by adding error handling:

```go
iter := ....
// Done() must never fail.origin/resource-management-docs
defer iter.Done()
// Next() must return false on error to break this loop
for iter.Next(&z) {
    ...
}
// Error handling should be performed after the loop
if err := iter.Err(); err != nil {
    ...
}
```

It is noteworthy that the `Iterable` interface is not changed and will still return an `Iterator`. This allows a type to *optionally* make the iteration safe by implementing the `SafeIterator`. The full `SafeIterator` interface definition is:

```go
type SafeIterator interface {
	Iterator
	SafetyAware

	BindThread(thread *Thread)
}
```

Given that it would be too error-prone to ask the user to check the type every time an iterator is used (and check the flags and bind the thread), a new convenience method similar to `starlark.Iterate` is added: `starlark.SafeIterate`. The main differences are:
 
 - its first argument is a `*starlark.Thread`, so that safety flags can be checked and the iterator can be bound to that thread. *This parameter can be nil*.
 - its return value includes an error, so that if some safety violation occurs, it can be handled immediately.

It's full usage patter is then:

```go
iter, err := starlark.SafeIterate(thread, iterable)
if err != nil {
    ...
}
// Done() must never fail.
defer iter.Done()
// Next() must return false on error to break this loop
for iter.Next(&z) {
    ...
}
// Error handling should be performed after the loop
if err := iter.Err(); err != nil {
    ...
}
```

[^1]: builtins are also used for performance, since they are compiled code and not interpreted instruction on a virtual processor.

[^2]: this could be the case of an object returned as a result from a script. It's still necessary to be able to inspect it or update it, while the execution of starlark has already ended.

[^3]: in fact, imagine a struct which implements both `SafeStringer` and `SafeIndexable` with different safety per operation. How whould the interpreter understand which flags to use if a single `Safety` method was present?

[^4]: most likely, the implementation of a method `XXX(...)` is just `return SafeXXX(nil, ...)`.
