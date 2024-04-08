# How to make a builtin safe

Starlark functionality can be enchanced by writing new functions in the Go language, usually called *builtin*. When developing a Starlark builtin, it is important to properly track memory usage to comply with safety requirements.

Every part of Starlark computation can be characterized by one or more safety aspects, which are expressed as `starlark.SafetyFlags`:
 - `starlark.CPUSafe`: the function is capable of counting the number of steps it performs and to stop if those steps are over the budget.
 - `starlark.MemSafe`: the function is capable of counting the number of bytes it allocates and to stop if those steps are over the budget.
 - `starlark.TimeSafe`: the function is capable of stopping, if requested, within a reasonable amount of time.
 - `starlark.IOSafe`: the function does not access any IO resource outside of the confinement requirements.

Starlark execution is always performed against a `starlark.Thread` object which acts as an *execution context*. The threads also contains the safety limits for a given execution, including:
 - allowed safety flags[^safety-flags];
 - memory allocation budget;
 - computation steps budget;
 - cancellation management.

[^safety-flags]: any part of the script execution which is not allowed will result in the termination of the run with an error.

## Declaring safety

The first step to provide a new builtin to the language is to *declare* it. This is achieved in upstream starlark with `starlark.NewBuiltin`, for example:

```go
beAwesomeBuiltin := starlark.NewBuiltin("be_awesome", beAwesome)
```

By default, this method creates a builtin which is marked as `NotSafe`. It is possible to use the method `DeclareSafety` to change that, for example in the `init` module function:

```go
func init() {
    beAwesomeBuiltin.DeclareSafety(starlark.MemSafe | starlark.IOSafe)
}
```

Flags can be combined with the bitwise or operator `|`. As a convention, the order of the flags usually follows their value (`CPUSafe`, `MemSafe`, `TimeSafe`, `IOSafe`).

A more compact way to declare a builtin's safety is to use the function `starlark.NewBuiltinWithSafety`, which combines the two steps above in a single one:

```go
beAwesomeBuiltin := starlark.NewBuiltinWithSafety("be_awesome", starlark.MemSafe | starlark.IOSafe, beAwesome)
```

While the latter approach is the preferred one for new code, when forking an existing library for use in the constrained starlark language, the former can be used to reduce merge claches with upstream.

## Counting memory usage

Starlark provides two methods to account for memory: 
 - `Thread.AddAllocs(allocs int64) error`: add allocs bytes to the used memory counter. If the operation would go over the budget, it fails with an error. `allocs` can be negative.
 - `Thread.CheckAllocs(allocs int64) error`: returns an error if a call to `AddAllocs` with the same amount would fail. It doesn't update the used memory counter.

Normally, it is difficult to understand when and if Go allocates memory by just reading the code as inlining and escape analisys can drastically change the memory layout. However, when used in the Starlark interpreter[^1], it can be assumed that:
 - the function will never be inlined;
 - the result of the function will always escape.

This simplifies greatly the complexity of writing memory accounting for a Starlark function.

Clearly, once allocated, the lifetime of a variable is not anymore under control of the function which can make no assumptions about it. However, when considering safety, it is ok to take a pessimistic approach, always taking into account the worst case. For memory, this means considering all allocations as lasting until the end of the script run. While this guarantees safety, it is also useful to distinguish between two types of allocation:
 - persistent allocations: all allocations which are reachable when the function ends;
 - transient allocations (memory spikes): all allocations which are not reachable (i.e. are collectable) when the function returns.

### Estimating allocation size

There are many ways Go allocates memory:
 - when a pointer escapes its context.
 - when a value is wrapped in an interface.
 - when a slice is allocated with `make`[^no-make].
 - when a new element is appended to a slice with no capacity left.
 - when a new element is inserted in a map.

[^no-make]: while this is in general true, if the size of the slice is fixed at compile time and the result does not escape, the Go compiler *might* replace the heap allocation with a stack one.

In general, it is difficult to compute precisely the amount of memory used as it sometimes depend on the content of the result and the state of the allocator. As such, we refer to the computation of the size of an objec as *estimating* the size. Starlark provides two functions to help with that: `starlark.EstimateSize` and `starlark.EstimateMakeSize`.

#### Estimating objects

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

#### Estimating slices

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

### Transient allocations

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

### Testing

[^1]: this does not mean that those function will never be inlined. If called directly in any other part of the codebase, they might.