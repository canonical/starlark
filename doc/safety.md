# How to make a builtin safe

Starlark's capabilities are enhanced by exposing new builtins (functions callable from Starlark, written in Go) or new types (types implementing Starlark interfaces, written in Go). Clearly, allowing users unlimited access to run code on the host can be problematic, hence safety constraints must be enforced. As this may only be done on a best-effort basis, safety can only be enforced with a contract - the host declares the safety properties it cares about and all code run on it must then respect these properties. The contract is upheld through testing.

Safety properties are specified through the use of starlark.SafetyFlags. These are:
 - `CPUSafe` - when significant work is done, such a function will count the number of arbitrary 'steps' and compare this to its budget.
 - `MemSafe` - when persistent or significant transient allocations are made, such a function will count the number of bytes used and compare this to its budget
 - `TimeSafe` - when the executing thread is cancelled, such a function will stop within a reasonable amount of time
 - `IOSafe` - such a function does not access any IO resource outside of confinement requirements.

Starlark execution is always performed with respect to a `starlark.Thread` object which acts as an *execution context*. This contains the safety constraints for a given execution, including:
 - required safety flags;
 - memory allocation budget;
 - computation steps budget;
 - cancellation token management.

## How to declare safety

<!-- Move NewBuiltinWithSafety first  -->

The first step to provide a new builtin to the language is to *declare* it. This is achieved in upstream starlark with `starlark.NewBuiltinWithSafety`, for example:

```go
beAwesomeBuiltin := starlark.NewBuiltinWithSafety("be_awesome", starlark.MemSafe | starlark.IOSafe, func(...) { ... })
```

As a convention, the order of the flags usually follows their value (`CPUSafe`, `MemSafe`, `TimeSafe`, `IOSafe`).

### How to reduce merge-conflicts with an upstream

When dealing with an upstream library which is not using safety machinery, it is often important to keep the patch size small and to avoid merge conflicts when possible. In this case, it is handy to not change the declaration, but separately declare the safety during initialization with the method `DeclareSafety`. For example:

```go
upstreamBuiltin := starlark.NewBuiltin("upstream", func(...) { ... })
...
func init() {
    upstreamBuiltin.DeclareSafety(starlark.MemSafe | starlark.IOSafe)
}
```

## How to count memory usage

Starlark provides two methods to account for memory: 
 - `thread.AddAllocs`: add the parameter to the used-memory counter. If the operation would go over the budget, it returns an error.
 - `thread.CheckAllocs`: returns an error if a call to `AddAllocs` with the same amount would fail. It doesn't update the used-memory counter.

Normally, it is difficult to understand when and if Go allocates memory by just reading the code as inlining and escape analysis can drastically change the memory layout. However, when used in the Starlark interpreter, it can be assumed that:
 - the function will never be inlined;
 - the result of the function will always escape.


There are many ways Go allocates memory:
 - when a pointer escapes its context.
 - when creating a slice with `make`[^no-make]
 - when appending to a slice with no remaining capacity
 - when inserting into a map
 - when converting any concrete value to an interface

NB: the last one is implicit!

Counting the exact amount of memory in use by a program at one time is both prohibitively complex and is little better than a good approximation. To this end, we partition declarations into two categories:

 - Persistent allocations - those still reachable after a builtin terminates.
 - Transient allocations - those which may be freed when the builtin terminates.

Persistent allocations must be counted. Transient allocations need only be counted if they are significantly large. (Small memory spikes are generally not worth the complication of counting.)

Given the expected infrequency of garbage collection cycles and the short-lived nature of safe Starlark execution, the freeing of persistent values can be ignored.

### How to estimate allocation size

The amount of memory used by a particular object can be estimated using the `starlark.EstimateSize` function. In the case of make, it is possible to estimate the amount of memory being used ahead of time using `starlark.EstimateMakeSize`.

#### How to estimate objects

`EstimateSize` takes an object and returns the estimated size of the whole object tree. As such, it usually forces the code to first allocate and then count for the memory[^fixed]. Moreover, when using `EstimateSize`, care should be taken in not counting objects more than once. For example:

```go
a, err := MakeObjSafe(thread) // Counts the cost of its result
if err != nil { ... }
b := MyStruct{ field: objA }
bSize := starlark.EstimateSize(b)
if err := thread.AddAllocs(bSize); err != nil { ... }
```

In the above example, the cost of `a` is being counted twice: once in `MakeObjSafe` and once when computing `bSize`. In these cases it is useful to pass to `EstimateSize` an *object template* instead, which does not reference the field:

```go
a, err := MakeObjSafe(thread) // Counts the cost of its result
if err != nil { ... }
bSize := starlark.EstimateSize(MyStruct{}) // empty template
if err := thread.AddAllocs(bSize); err != nil { ... }
b := MyStruct{ field: objA }
```

A nice side effect of this pattern is that the allocation check can be finer-grained and can be moved *before* the allocation happens. As such, the object template is also useful when the size of the object does not depend on its content.

`EstimateSize` is also capable of estimating the size of `chan`nels `map`s and `slice`s. In the first case, it only takes into account the size of the channel buffer. In the other cases, all keys and all values will be taken into account, making the operation rather expensive. Moreover, in the case of `map` only the size can be taken into account, making the estimation sometimes unreliable.

#### How to estimate slices

Recognizing a slice allocation is rather straightforward as it is literally calling the `make` builtin:

```go
storage := make([]int, n)
```

The amount of memory necessary to satisfy this allocation can be easily estimated with a call to `EstimateMakeSize`:

```go
storageSize := starlark.EstimateMakeSize([]int{}, n)
```

However, when dealing with interfaces, converting a value to an interface might require additional memory. Let's consider the following code:

```go
result := make([]any, n)
for i := 0; i < n; i++ {
    result[i] = i
}
```

One might be tempted to make this function safe by simply adding:

```go
resultSize := starlark.EstimateMakeSize([]any{}, n)
if err := thread.AddAllocs(resultSize); err != nil { ... }
```

However, testing the result might be surprising as it would fail. In fact, each element of `result` needs some memory to store the value (an integer in this case) in the interface. The need for this allocation is somehow subtle and depends *on the type of `result`* and not on the type of `i`.

It is possible to ask `EstimateMakeSize` to take this kind of scenarios into account by specifing a *template* for the element, in a similar way a template is used for `EstimateSize`. In this case, for example, to estimate the real size of `result` it is possible to write:

```go
resultSize := starlark.EstimateMakeSize([]any{int(0)}, n)
```

The `int(0)` serves as the *element template* and will be taken into account for the estimation of the size.

As a last note, when returning parts of existing strings or slices, the backing array is shared, so no allocation takes place for that. However, when converting the string or slice to an interface, a small header needs to be allocated. For these cases, the `starlark` package provides two constants to easily count for this cost: `StringTypeOverhead` and `SliceTypeOverhead`.

### How to constrain transient allocations

The main objective when dealing with a transient allocation is to make sure that the spike is contained, to avoid spikes so big that might take the whole embedding application down.

There are two ways to deal with transient allocations in Starlark. The easiest and natural one is to use `CheckAllocs` method to ask the thread if there is enough memory for the spike. Even when available, no memory will be added to the memory budged. For example:

```go
allocationSize := 300 * 1024 // 300 KiB
if err := thread.CheckAllocs(allocationSize); err != nil {
    return nil, err
}
storage := make([]byte, allocationSize)
```

This, however, would not work if two or more transient allocation are done separately, for example:

```go
thread.SetMaxAllocs(1000)
...
if err := thread.CheckAllocs(900); err != nil { // OK, budget is enough
    return nil, err
}
storage1 := make([]byte, 900)
if err := thread.CheckAllocs(600); err != nil { // Not ok, it should fail!
    return nil, err
}
storage2 := make([]byte, 600)
```

In this case, the allocation made was about `1500` bytes which is above the budget. However, the logic failed to detect it as `CheckAllocs` does not update the counter. For this reason, it is ok in these cases to call `AddAllocs` instead, with a negative amount. A possible solution in the case above would be:

```go
thread.SetMaxAllocs(1000)
...
if err := thread.AddAllocs(900); err != nil {
    return nil, err
}
storage1 := make([]byte, 900)
if err := thread.AddAllocs(600); err != nil {
    return nil, err
}
storage2 := make([]byte, 600)
...
if err := thread.AddAllocs(-1500); err != nil {
    return nil, err
}
```

### How to test memory safety
