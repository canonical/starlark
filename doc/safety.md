# How to make a builtin safe

Starlark's capabilities are enhanced by exposing new builtins (functions callable from Starlark, written in Go) or new types (types implementing Starlark interfaces, written in Go). Clearly, allowing users unlimited access to run code on the host can be problematic, hence safety constraints must be enforced. Unfortunately, the Go interpreter doesn't offer a good basis for such hard enforcement of constraints, so what is implemented is a best effort system via a contract: the host declares the safety properties it cares about and all code run on it must then respect these properties. The contract is upheld through testing.

Safety properties are specified through the use of starlark.SafetyFlags. These are:
 - `CPUSafe`: when significant work is done, such a function will count the number of arbitrary 'steps' and compare this to its budget.
 - `MemSafe`: when persistent or significant transient allocations are made, such a function will count the number of bytes used and compare this to its budget
 - `TimeSafe`: when the executing thread is cancelled, such a function will stop within a reasonable amount of time
 - `IOSafe`: such a function does not access any IO resource outside of confinement requirements.

Starlark execution is always performed with respect to a `starlark.Thread` which acts as an *execution context*. This contains the safety constraints for a given execution, including:
 - required safety flags;
 - memory allocation budget;
 - computation steps budget;
 - cancellation token management.

## How to declare safety

The first step to provide a new builtin to the language is to *declare* it. This is achieved in upstream starlark with `starlark.NewBuiltinWithSafety`, for example:

```go
beAwesomeBuiltin := starlark.NewBuiltinWithSafety("be_awesome", starlark.MemSafe | starlark.IOSafe, func(...) { ... })
```

As a convention, the order of the flags usually follows their value (`CPUSafe`, `MemSafe`, `TimeSafe`, `IOSafe`).

### How to reduce merge-conflicts with an upstream library

When dealing with an upstream library which is not using safety machinery, it is often important to keep the patch size small and to avoid merge conflicts when possible. In this case, it is handy to not change the declaration, but separately declare the safety during initialization with the method `DeclareSafety`. For example:

```go
upstreamBuiltin := starlark.NewBuiltin("upstream", func(...) { ... })
...
func init() {
    upstreamBuiltin.DeclareSafety(starlark.MemSafe | starlark.IOSafe)
}
```

## How to count memory usage

Two methods are provided to account for memory:
 - `thread.AddAllocs`: add the parameter to the used-memory counter. If the operation would go over the budget, this method returns an error.
 - `thread.CheckAllocs`: check whether adding the parameter to the used-memory counter would return an error. This method doesn't update the used-memory counter.

Normally, it is difficult to understand when and if Go allocates memory by just reading the code as inlining and escape analysis can drastically change the memory layout. However, when used in the Starlark interpreter, it can be assumed that:
 - the function will never be inlined;
 - the result of the function will always escape.

There are many ways Go allocates memory:
 - when a pointer escapes its function scope
 - when converting any concrete value to an interface
 - when creating a slice with `make`
 - when appending to a slice with no remaining capacity
 - when inserting into a map

NB: the first two are implicit!

Counting the exact amount of memory in use by a program at one time is both prohibitively complex and little better than a good approximation. To this end, we partition declarations into two categories:

 - Persistent allocations---those still reachable after a builtin terminates.
 - Transient allocations---those which may be freed when the builtin terminates.

Persistent allocations must be counted. Transient allocations need only be counted if they are significantly large (small memory spikes are generally not worth the complication of counting).

Given the expected infrequency of garbage collection cycles and the short-lived nature of safe Starlark execution, the freeing of persistent values can be ignored.

### How to estimate allocation size

In our API, there are two central functions and two central values which may be used to estimate the size of any Go value. These are:
 - `EstimateSize`, which estimates the size of a given value
 - `EstimateMakeSize`, which estimates the size to be allocated by a call to make with the same arguments
 - `StringTypeOverhead` and `SliceTypeOverhead`, which account for the top-level cost of these structures, useful when a string/slice is created which is just a sub-string/sub-slice of another, already accounted-for one

#### How to estimate values

So you've found an allocation and you want to account for it. What should you use?

`EstimateSize` takes a value and returns the estimated size of the whole tree. As such, it usually forces the code to first allocate the value and then count its memory, hence to avoid large spikes, code should be structured so that this is only called on relatively small values (which may make up a larger one). Exactly how is described later.

```go
if err := thread.AddAllocs(starlark.EstimateSize(myValue)); err != nil { ... }
```

When using `EstimateSize`, care should be taken in not counting values more than once. For example:

```go
a, err := MakeA(thread) // Expect this to count the cost of a
if err != nil { ... }
b := &B{ a }
bSize := starlark.EstimateSize(b) // a is also counted here!
if err := thread.AddAllocs(bSize); err != nil { ... }
```

To avoid this double-counting, pass an *value template* to `EstimateSize`. Here we define a value template as a partially-constructed instance of that type where only the fields we want to count are populated. In the above example, no fields will be populated in the new template.

```go
a, err := MakeA(thread) // Expect this to count the cost of a
if err != nil { ... }
bSize := starlark.EstimateSize(B{}) // empty template
if err := thread.AddAllocs(bSize); err != nil { ... }
b := B{ a }
```

A nice side effect of this pattern is that the allocation check can be finer-grained and can be moved *before* the allocation happens. As such, the template is also useful when the size of the value does not depend on its content, but just on its type.

Estimating the cost of an entire structure can be expensive, for example when a map, slice or array is passed, every key and every value will also be traversed, along with their children!

Although this function can slightly (and safely) overestimate, in the case of channels and maps it may also underestimate! As Go does not expose the content of channels for reflection, only the size of the channel buffer can be accounted for. Similarly, Go does not expose the capacity of a map, only its length, hence if a map is created, many items added then many removed, len(map) might be low, but its capacity could be much higher! Care must be taken if Starlark is given access to such values.

#### How to estimate slices

To recognise a slice allocation, look for either a call to the `make` builtin or an expression like `[]T{...}`. The amount of memory necessary to satisfy this allocation can be easily estimated with a call to `EstimateMakeSize`:

```go
size := starlark.EstimateMakeSize([]int{}, n)
if err := thread.AddAllocs(size); err != nil {...}
```

However, when dealing with interfaces, there is often an implicit conversion from a value to an interface, which might require additional memory. Let's consider the following code:

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

However this would be incorrect! The elements of `result` are not integers but interfaces *containing* integers and that implicit interface conversion forces an allocation, even for simple types. This allocation is subtle and depends on the type of `result` and not the type of `i`.

To make `EstimateMakeSize` take these kinda of scenarios into account, specify a template for the element*, in a similar way a template is used for `EstimateSize`. For the above case, to estimate correctly and concisely the size of `result` it is possible to write:

```go
resultSize := starlark.EstimateMakeSize([]any{int(0)}, n)
```

Always be careful to match the types passed in the template with exactly the types being handled, especially when interfaces are involved. Taking good care here will pay off when it comes to testing.

As a last note, when converting a slice to an interface, the space for its *header* is allocated. This holds true for strings as well. Given how common they are, two constants are provided these cases: `StringTypeOverhead` and `SliceTypeOverhead`.

### How to constrain transient allocations

Accounting for every single allocation Go makes during a computation is prohibitively complex, so some spikes in memory usage e.g. for scratch space when computing some value are somewhat inevitable. The key to making these safe is to prevent these spikes getting so large that they can take down the entire embedding application.

Easiest and most natural one is to use the `CheckAllocs` function to ask the current Starlark thread whether there is enough memory in its budget to account for the spike. The total counted memory remains unchanged in this case. For example:

```go
spikeSize := 300 * 1024 // 300 KiB
if err := thread.CheckAllocs(spikeSize); err != nil {
    return nil, err
}
scratchBuffer := make([]byte, allocationSize)
```

This, however, would not work if two or more transient allocation are declared separately. Consider the following example:

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

In this case, the *total* spike size was around `1500` bytes which is above the budget. However, this logic failed to detect it as `CheckAllocs` does not update the counter. To avoid this problem, use AddAllocs to update the counter, then AddAllocs again, this time with a negative amount, to remove the counting. This technique is especially important in recursive functions! A possible solution in the case above would be:

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
spikeSize := 900 + 600
if err := thread.AddAllocs(-spikeSize); err != nil {
    return nil, err
}
```

Constructs are provided to help count allocations in some [common cases](#common-patterns).

### How to test memory safety

All the above declarations are good in theory but to achieve safety we must test this model against reality. Don't be surprised if you've missed something.

Say we've written a builtin called `myAwesomeBuiltin`. To test its memory safety, use the `startest` package, create a new test instance as detailed below and make sure to *require* the `MemSafe` flag to tell the instance to test for memory safety.

```go
func TestMyAwesomeBuiltinAllocs(t *testing.T) {
    st := startest.From(t) // by convention, the startest instance is called `st`
    st.RequireSafety(starlark.MemSafe)
```

Then, use the `st.RunThread` method to make a workload to benchmark. This must allocate an amount of memory proportional to the provided `st.N`. Make sure this has no side-effects as this logic may be run many times. Finally, call `st.KeepAlive` with the value to be measured which in our case is the result of our builtin

```go
   st.RunThread(func(thread *starlark.Thread) {
        // (The existing code copy-pasted)
        for i := 0; i < st.N; i++ {
            result, err := starlark.Call(thread, beAwesomeBuiltin, nil, nil)
            if err != nil {
                st.Error(err)
            }
            st.KeepAlive(result)
        }
    })
}
```

This pattern of repeatedly calling the builtin st.N times works when the input has no effect on the size of the output. To exercise a function whose output size depends on its input, use `st.N` to construct an input which will force an output with size proportional to `st.N`.

```go
func repeat(thread *starlark.Thread, v starlark.Value, times int) []starlark.Value
...
func TestRepeatAllocs(t *testing.T) starlark.Tuple {
    st := startest.From(t)
    st.RequireSafety(starlark.MemSafe)
    st.RunThread(func(thread *starlark.Thread) {
        tuple := starlark.Tuple{ starlark.True, starlark.False, starlark.None }
        result = repeat(thread, tuple, st.N) // the output depends on st.N, no need for a loop here
        st.KeepAlive(result)
    })
}
```
As a final note, to check that function does not allocate at all, use `st.SetMaxAllocs(0)` to set the maximum permissible allocations per st.N to zero as in the following:

```go
func shouldNotAllocate(value starlark.Value) starlark.Value { return value }
...
func TestIdentityAllocs(t *testing.T) starlark.Tuple {
    value := ...
    st := startest.From(t)
    st.RequireSafety(starlark.MemSafe)
    st.SetMaxAllocs(0)
    st.RunThread(func(thread *starlark.Thread) {
        for i := 0; i < st.N; i++ {
            result = shouldNotAllocate(value)
            st.KeepAlive(result)
        }
    })
}
```

## How to count CPU usage

CPU usage is measured in terms of arbitrary steps.
Roughly speaking, if a significant amount of work is done, some steps must be counted.
The exact meaning of ‘significant’ here may depend on the context, for example tasks which take more than 1ms of CPU time may be considered significant.

Two methods are provided to account for steps:
 - `thread.AddSteps`: add the parameter to the step counter. If the operation would go over the budget, this method returns an error.
 - `thread.CheckSteps`: check whether adding the parameter to the step counter would return an error. This method doesn’t update the step counter.

Say we have a function which, if present, strips some prefix from a string:

```go
func stripPrefix(thread *starlark.Thread, toBeStripped, prefix string) (string, error) {
    if !strings.HasPrefix(toBeStripped, prefix) {
        return str, nil
    }
    return toStrip[len(prefix):], nil
}
```

It is clear that the `strings.HasPrefix` function must iterate through the string to check whether the prefix is present.
In the worst case, this will check `len(prefix)` characters which, if `prefix` is extremely long, may require a lot of work.
To prevent too much work being done, declare a number of steps proportional to the amount of work to be done just before doing it.
In this case, we may add the following lines are added before the `strings.HasPrefix` call:

```go
    if err := thread.AddSteps(int64(len(prefix))); err != nil {
        return "", err
    }
```

When `prefix` is non-empty, this will add a non-zero number of steps to the step counter, but what about when `prefix` is the empty string?
In this case, although some work will be done, the amount is insignificant so can be safely ignored as the Starlark interpreter will implicitly add at least one step every time a builtin is called.
Note that the number of steps is proportional to the work done and not the actual CPU usage---if some optimisation were to make this run significantly faster, the same number of steps would still be sufficient.
Our goal here is to stop runaway scripts, rather than to achieve extremely precise limitation, hence some approximation is fine.

Constructs are provided to trivialise step-counting when using [common cases](#common-patterns).

## How to test for CPU safety

To test the CPU usage of `stripPrefix`, use the `startest` package, create a new instance as detailed below and *require* the `CPUSafe` flag to tell the instance to test for CPU usage.

```go
func TestStripPrefixSteps(t *testing.T) {
    st := startest.From(t)
    st.RequireSafety(startest.CPUSafe)
```

To check that our step model is being followed, set the minimum and maximum number of steps allowed per `st.N`.
Soon, a test case which uses exactly one step per `st.N` will be created.

```go
    st.SetMinSteps(1)
    st.SetMaxSteps(1)
```

Then, declare the workload to benchmark using the `st.RunThread` method.
Note that unlike the memory tests above:
 - `st.KeepAlive` is not required
 - the amount of work done by the test is scaled by constructing an input of length `st.N`
The latter removes the need for a `for` loop which runs `st.N` times.
```go
    st.RunThread(func(thread *starlark.Thread) {
        toBeStripped := strings.Repeat("a", st.N)
        prefix := strings.Repeat("a", st.N)
        args := starlark.Tuple{toBeStripped, args}
        _, err := starlark.Call(thread, stripPrefix, args, nil)
        if err != nil {
            st.Error(err)
        }
    })
}
```

## Common patterns

The following recurring patterns have been observed when adding safety bounds.
Helper functions and types are provided for each.

### How to safely build a string

To safely build a string, use `SafeStringBuilder`, which functions as a safe replacement for `strings.Builder`.
When using it, always remember to check the result of the `Err()` immediately before calling the `String()` method.

Usage of `SafeStringBuilder` matches that of `strings.Builder`, for example, a snippet to safely format a log message may look something like this:

```go
// Format string as "[timestamp]: message"
buf := starlark.NewSafeStringBuilder(thread)
if _, err := buf.WriteByte('['); err != nil {
    return nil, err
}
if _, err := buf.WriteString(timestamp.Format(time.RFC3339)); err != nil {
    return nil, err
}
if _, err := buf.WriteString("]: "); err != nil {
    return nil, err
}
if err := buf.Err(); err != nil {
    return nil, err
}
return buf.String(), nil
```

The `SafeStringBuilder` will both count the allocations for the string it constructed and the steps required to do so.

### How to safely iterate

To safely iterate over a `Value` use `SafeIterate`, which functions as a safe replacement for `Iterate`.
When using it, always remember to call the `Err()` method once iteration is complete.

For example, to safely iterate over a structure, use the following snippet:

```go
iter, err := SafeIterate(thread, iterableValue)
if err != nil {
    return nil, err
}
defer iter.Done()
var x Value
for iter.Next(&x) {
    // ...
}
if err := iter.Err(); err != nil {
    return nil, err
}
return True, nil
```

### How to safely append a single item to a slice

To safely append to a slice, wrap it in a `SafeAppender` and use its `Append` method.
When using it, note that the `SafeAppender` modifies the slice in-place.

For example, to safely perform `mySlice = append(mySlice, value)`, use the following snippet:

```go
mySliceAppender := NewSafeAppender(thread, &mySlice)
if err := mySliceAppender.Append(value); err != nil {
    return nil, err
}
```

### How to safely concatenate two slices

To safely concatenate one slice to another, wrap it in a `SafeAppender` and use its `AppendSlice` method.
When using it, note that the `SafeAppender` modifies the slice in-place.

For example, to safely perform `mySlice = append(mySlice, other...)`, use the following snippet:

```go
mySliceAppender := NewSafeAppender(thread, &mySlice)
if err := mySliceAppender.AppendSlice(values); err != nil {
    return nil, err
}
```
