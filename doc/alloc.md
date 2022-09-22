
# Allocation overview

This document tries to list all the possible allocations performed by the Starlark byte-code interpreter instruction by instruction, without taking into account builtin functions.

The term "allocation" should be taken as an approximation since in Go it is hard to decide whether or not an instantiation actually results in a heap allocation (even when the variable does escape). Moreover, the allocations discussed here will not take into account the space required for Go to represent the `Value` interface.

## NOP
    
Of course, no allocation occurs during a no-op.

## DUP, DUP2

These instruction duplicate the last one (DUP) or two (DUP2) values from the top of the stack. Values are interfaces. Copying an interface should not need an allocation (it \[possibly\] already allocated during the interface creation).

## POP

This instruction does not allocate.

## EXCH

This instruction does not allocate.

## Binary comparisons (EQL, NEQ, GT, LT, LE, GE)

This function returns a new boolean value wrapped in an interface [^one-byte].

[^one-byte]: value types whose size is 1 byte or whose value is binary 0 usually don't result in an allocation due to an optimization used by the compiler (and given its value semantic). Still, this is an implementation detail that I think should not be considered in this stage.

In the default library no allocation occurs in any of the implementations of the `Comparable` interface (except for the one described below). User-defined comparisons need to be assessed individually.

While counterintuitive, there is one case where comparing two `Values` will use memory: float/integer comparisons(!) [^float-compare].

[^float-compare]: I believe that it is generally a bad idea to compare integers with floats. It only makes sense for constants since there is no guarantee that the result of a floating-number operation is exact, even when mathematically it should (e.g. `(sin^2(x) + cos^2(pi)) == 1` will often be `false`). Moreover the value must be below 2^53 since over 2^53 you don't know if what is stored in the float is the result of an approximation or that very number (e.g. `18014398509481986.0 == 18014398509481984` is `true` in Starlark).

The idea behind this comparison is to transform both numbers in a `big.Rat` rational number, which is made of two `big.Int`. The maximum size of this comparison can be computed statically for the float (since the max module is `1.79769313486231570814527423731704356798070e+308`) and dynamically for the integer.

## PLUS

Depending on the types involved this instruction could lead to allocations. The result is stored in a `Value` so in case of value types it will require an allocation.

### String + String

This will perform string concatenation and result in an allocation of at least `Len(lhs) + Len(rhs)` bytes.[^empty]

[^empty]: the compiler could however decide to not allocate a string if `lhs` or `rhs` is empty. We assume this doesn't happen.

### Int + Float, Float + Int, Float + Float

This will not result in any additional allocation.

### Int + Int

This will result in an allocation if any of the two numbers is a `big.Int` or the result of the sum cannot be represented in 32bits.

We can distinguish three different cases:
 - both are `big.Int`: a single `big.Int` will be allocated.
 - one is a `big.Int`, the other is a `int64`. The `int64` will be transformed to a `big.Int` which will result in an allocation. Its worst-case *minimum* size in bits should be 64.
 - both are `int64`. No additional allocation occurs.

If the result is a `big.Int`, it's minimum size in bits should be less than `max(bitLen(lhs), bitLen(rhs)) + 1`, but the implementation of `big.Int` can allocate more to avoid reallocation during normal usage (we **are not** in normal operation since `big.Int` uses a reference semantics while Starlark uses value).

### List + List, Tuple + Tuple

In both cases, a new slice of minimum capacity `len(lhs) + len(rhs)` will be allocated. The elements are of (`Value`) interface type, so no allocation during assignment.

## MINUS

Minus is defined only for integers and float (or a mix of them). Same rules as [Plus](#plus).

## STAR

Just like other binary operators, allocation can occur depending on the types involved.

### Int * Int

Similar to [plus](#plus) except that the minimum size of the result (in bits) in the case of a `big.Int` is `bitLen(lhs) + bitLen(rhs)`.

### Int * Float, Float * Int, Float * Float

This will not result in any additional allocation.

### Int * String, String * Int, Int * Bytes, Bytes * Int

Both `String` and `Bytes` are alias to Go type `string`. For this reason, they also share the implementation (and thus the space requirements).

This, together with the `Tuple`/`List` one, is likely the most dangerous operator in Starlark as a single instruction could cause *gigabytes* of allocated data. The implementation currently caps this at 1 GB. Still a lot.

The minimum (and likely) amount of bytes that the string buffer will take  (e.g. without taking int account fields like length and/or any other low-level detail) is thus given by `min(1 << 30, max(0, rep) * len(str))` where `rep` is the `Int` parameter and `str` is the `String`/`Bytes` one.

### Int * List, List * Int, Int * Tuple, Tuple * Int

Similar to the string method, it will also cap the number of elements in a list to `2^30`. 

The minimum capacity of the resulting slice is thus given by `min(1 << 30, max(0, rep) * len(list))` where `rep` is the `Int` parameter and `list` is the `List`/`Tuple` one. The values with which is filled were already of interface type `Value`, so no allocation should result there. Still this could easily result in a very big buffer.

## SLASH

This operator is defined only for combinations of `Int` and `Float` types. The result is `Float`, so it will never allocate (except for the returned `Value`).

## SLASHSLASH

This operator is defined only for combinations of `Int` and `Float` types. It will incur in an allocation only when both types are `Int`.

In this case, only if at least one of the operand is an `big.Int` there will be allocations:

 - if one of the operands is an int64, than it will be transformed in an `big.Int` (with worst-case *minimum* length of 64 bits);
 - it will than allocate two `big.Int`s one for the quotient and one for the remainder. The minimum length of those two should be linked by `bitLen(quo) + bitLen(rem) < bitLen(lhs) + 2` and also `bitLen(rem) <= bitLen(rhs)` (not very informative though...)

If the result cannot be represented in 32bits, there will be another allocation (for no apparent reason other than code reuse).

## PERCENT

### Int and Float

From the point of view of the allocations, the patter is very similar to [SLASHSLASH](#slashslash), but instead of duplicating the quotient, it will duplicate the remainder (at the end).

### String

String formatting is very tricky to track from the allocations POV. The argument can be:
 - a `Mapping` interface;
 - a `Tuple` type;
 - a `Value` (which is treated as a `Tuple` of size 1)

Allocations start with a `strings.Builder` which is a buffer that grows quadratically to achieve insertion time of `O(1) (amortized)`. This could result in a higher number of allocations than expected (and, given the implementation, it would probably be hard to tackle). It is probably easier to just think of it as it always allocates the perfect amount of bytes (maybe rounded to the next power of 2).

The following subsections try to discuss allocations other than the bytes written in the builder.

#### Key syntax

In this case, it will be also called the interface method `Get(key Value) (v Value, found bool, err error)`. In the default implementation (`Dict`) this won't allocate, but a different implementation might [^hash].

[^hash]: `(Dict) Get(...)` calls `(Value) Hash()` which might allocate as well (strange though).

#### Formats `s` and `r`

This will transform the argument in a string by recursively traversing the `Value` itself. `List`s, `Dict`s and `Tuple`s will be expanded. It is going to be hard to track these allocations. Moreover, it internally uses `fmt` library, which could allocate itself.

TODO: improve

#### Formats `d`, `i`, `o`, `x` and `X`

This will transform the input in an `Int`. If the argument is a `Float` it will be transformed to an `Int`. This could cause allocations if the value is big enough (e.g. not representable in 32bits).

#### Formats `e`, `f`, `g`, `E`, `F` and `G`

No allocation directly occurs (without taking int account the internals of `fmt`, if any) except for formats `g` and `G` which allocate a separate string instead of using the builder directly.

#### Format `c`

No allocations. It will write 4 bytes at most.

## AMP

### Int & Int

Similarly to other Int operations, if at least one of the operands is an `big.Int`, all the arguments will be converted to `big.Int`, leading to an allocation. 

If necessary the result will be allocated. The maximum size of the result in bits is `min(bitLen(lhs), bitLen(rhs))`.

### Set & Set

Set will create a new hashtable and insert elements found in both one by one. Allocation is hard to predict, but its maximum size (e.g. the elements contained, not the capacity or the bucket number) is `min(count(lhs), count(rhs))`.

## PIPE


### Int | Int

Same as [AMP](#amp), but the result size in bits is `max(bitLen(lhs), bitLen(rhs))`.

### Set | Set

Pipe will allocate an Iterator only for the `rhs`. The resulting set will have at most `count(lhs) + count(rhs)` elements.

## CIRCUMFLEX

### Int ^ Int

Same as [AMP](#amp), but the maximum result size in bits is `max(bitLen(lhs), bitLen(rhs))`.

### Set ^ Set

No allocations except the result. The resulting set will have at most `count(lhs) + count(rhs)` elements.

## LTLT

Only among integers. `rhs` must be positive and less than 512. The minimum size in bits of the result is `bitLen(lhs) + rhs`.

## GTGT

Only among integers. `rhs` must be positive. The minimum size in bits of the result is `bitLen(lhs) - rhs`.

## IN, NOT_IN

No allocation, but it calls `Mapping.Get`. In case of user-defined type this could allocate (why?).

## User-defined binary operations

Can have problems given that it cannot access the current `Thread`.

### Time/Duration

No allocation occurs in the `Time`/`Duration` binary operations.

### Struct

TODO

## UPLUS, UMINUS, TILDE

### Int

UPLUS will return the very same reference (makes sense given the value semantics). Other will allocate another `big.Int` if required

### Float

No allocation.

### INPLACE_ADD

This will behave just like [PLUS](#plus), except for `List`s. The list will be extended instead of creating a new list with all the elements (as in [PLUS](#plus)).

From the point of view of allocations, the resulting slice will have `len(rhs)` elements more (possibly re-allocating the backing array). Moreover, it may use the `Iterable` interface which will incur in allocations:

 - `Iterable.Iterate()` will allocate an `Iterator` interface;
 - `Iterator.Next` will allocate if the result is a value type;

All these methods could allocate more in case of a custom implementation.

## NONE, TRUE, FALSE, MANDATORY (push respectively `None`, `True`, `False` and `mandatory` in the stack)

No allocation since `None`, `True` and `False`, `mandatory` are constants. 

## JMP

No allocation during a JMP.

## ITERPUSH

It will allocate an iterator. Usually a lightweight object. Custom implementations may vary.

## ITERJMP

Custom implementations may allocate more. This could be problematic as `Next` cannot access `Thread`.

## ITERPOP

Custom allocations may allocate.

## NOT

Not could incur in allocation only if a `Value` allocates in its `Truth() bool`. No types in the default library (including `time`, `json`, `math` and `proto`) allocate in any `Truth` function.

## RETURN

No allocation for return statement.

## SETINDEX


### HasSetKey

In case of a `Dict` this could lead to rehashing (potentially more than once). User-defined types could allocate more. `SetKey` cannot access the current `Thread`.

### `List`s

In case of `List` no allocation occurs.

## INDEX

It will call `Mapping.Get` or `Indexable.Index`. User-defined types could allocate.

## ATTR

`String`, `List`, `Set`, `Dict` and `Bytes` types will return a bound function (closure) and thus will need allocation.

Other will just return the `Value`. 

TODO: `proto`

## SETFIELD

Custom types may allocate.

TODO: `proto`


## MAKEDICT

This will allocate an empty dictionary (struct only).

## SETDICT, SETDICTUNIQ

May result in (multiple) rehashing.

## APPEND

Append will result in an allocation if the capacity of the list is not enough to append new element.

## SLICE

All `slice`s will allocate a new go slice to store the result, except for `Bytes`, `Tuple` and `String` that (being immutable) will result in a reference to the original object if `step` is `1`.

Custom implementations of `Sliceable` may vary.

## UNPACK

Unpack will allocate a `Iterator` interface and a `Value` for each value-type object enumerated.

It will call `Iterable.Iterate()`, `Iterable.Next()` and `Iterable.Done()`. All of them could allocate in a custom type.

## CJMP

Conditional jumps could incur in allocation only if a `Value` allocates in its `Truth() bool` function. No types in the default library (including `time`, `json`, `math` and `proto`) allocate in any `Truth` function.

## MAKETUPLE, MAKELIST

Allocate a slice of size `n`.

## MAKEFUNC

Will just allocate the (`Function`) `Value`. All the arguments and freevars must already be allocated on the stack as a `Tuple` (that's where the real allocation occurs).

## LOAD

Load is implementation-defined. It will most likely allocate, but it is not possible to estimate in the interpreter.

## SETLOCAL, SETLOCALCELL, SETGLOBAL, LOCAL, FREE, LOCALCELL, FREECELL♣️, GLOBAL, PREDECLARED, UNIVERSAL, CONSTANT

All these assignments are copying interfaces (of type `Value`) so no allocation here.

## CALL, CALL_VAR, CALL_KW, CALL_VAR_KW

Call will allocate:

 - 1 slice of `Tuple`s of size `arg & 0xff` and another one of size `2 * nkvpairs` to hold named arguments wheb passed on the stack (`CALL_KW`)
 - 1 slice of `Tuple`s of size `kwargs.Len()` + 1 slice of `Value`s of size `2 * kwargs.Len()` to hold named arguments when passed by var (`CALL_VAR_KW`)
 - 1 `Tuple` of size `arg >> 8` for positional arguments (`CALL`), only if the calle is not a `Function` (otherwise it uses the stack in-place)
 - 1 `Tuple` of size `sizeof(args)` for positional arguments (`CALL_VAR`).

All those allocations should escape since their size is not known at compile time (and possibly "large" from a stack POV).

In case of `CALL_VAR_KW`/`CALL_VAR` it also uses the `IterableMapping`/`Iterable` interface respectively, which will be probably resolved run-time and will probably result in some other allocation.

`Call` function will internally allocate a frame, but only if it wasn't already allocated (so up to `N` frames where `N` is the maximum call depths of a starlark program).

In case of a starlark `Function` it will also allocate a `Value` slice to account for required stack and locals.

Other `Callable` implementation might allocate more.
