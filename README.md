# Starlark in Go

[![Go Tests](https://github.com/canonical/starlark/actions/workflows/tests.yml/badge.svg)](https://github.com/canonical/starlark/actions/workflows/tests.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/canonical/starlark.svg)](https://pkg.go.dev/github.com/canonical/starlark)

This is a fork of Google's _Starlark in Go_ project, maintained by Canonical, which adds safety constraints to the language.
It is developed to be a mostly-drop-in replacement for upstream Starlark, with a small number of reasonable [breaking changes](#breaking-changes).

Starlark in Go is an interpreter for Starlark, implemented in Go.
Starlark was formerly known as Skylark.

Starlark is a dialect of Python intended for use as a configuration language.
Like Python, it is an untyped dynamic language with high-level data
types, first-class functions with lexical scope, and garbage collection.
Unlike CPython, independent Starlark threads execute in parallel, so
Starlark workloads scale well on parallel machines.
Starlark is a small and simple language with a familiar and highly
readable syntax. You can use it as an expressive notation for
structured data, defining functions to eliminate repetition, or you
can use it to add scripting capabilities to an existing application.

A Starlark interpreter is typically embedded within a larger
application, and the application may define additional domain-specific
functions and data types beyond those provided by the core language.
For example, Starlark was originally developed for the
[Bazel build tool](https://bazel.build).
Bazel uses Starlark as the notation both for its BUILD files (like
Makefiles, these declare the executables, libraries, and tests in a
directory) and for [its macro
language](https://docs.bazel.build/versions/master/skylark/language.html),
through which Bazel is extended with custom logic to support new
languages and compilers.

## Differences with Google's Starlark in Go

### Additions

- [Safety constraints and helpers](doc/safety.md) to abide by them
- [`Context`](https://pkg.go.dev/context) integration with the execution `Thread`
- Project-wide overflow protection
- Fallable iterators with the new `Err()` method

### Breaking changes

- The maximum Starlark stack depth is now limited to avoid stack overflows. The limit is extremely high so will likely not affect any reasonable recursive application
- `thread.Uncancel` has been removed as it is exceedingly difficult to uncancel a thread in a concurrent environment whilst still maintaining safety guarantees
- `thread.Cancel` now accepts format args, allowing errors to be wrapped. The type of this function has changed different; existing calls are unaffected
- `thread.OnMaxSteps` has been removed to discourage changing limits

## Documentation

* Language definition: [doc/spec.md](doc/spec.md)

* About the Go implementation: [doc/impl.md](doc/impl.md)

* About safety constraints: [doc/safety.md](doc/safety.md)

* About breaking changes with upstream: [doc/breaking.md](doc/breaking.md)

* API documentation: [pkg.go.dev/github.com/canonical/starlark](https://pkg.go.dev/github.com/canonical/starlark)

* Issue tracker: [github.com/canonical/starlark/issues](https://github.com/canonical/starlark/issues)

### Getting started

Build the code:

```shell
# check out the code and dependencies,
# and install interpreter in $GOPATH/bin
$ go install github.com/canonical/starlark@latest
```
<!-- TODO(kcza): update the package version above -->

Run the interpreter:

```console
$ cat coins.star
coins = {
  'dime': 10,
  'nickel': 5,
  'penny': 1,
  'quarter': 25,
}
print('By name:\t' + ', '.join(sorted(coins.keys())))
print('By value:\t' + ', '.join(sorted(coins.keys(), key=coins.get)))

$ starlark coins.star
By name:	dime, nickel, penny, quarter
By value:	penny, nickel, dime, quarter
```

Interact with the read-eval-print loop (REPL):

```pycon
$ starlark
>>> def fibonacci(range(n))
...    for i in res[2:]:
...        res[i] = res[i-2] + res[i-1]
...    return res
...
>>> fibonacci(10)
[0, 1, 1, 2, 3, 5, 8, 13, 21, 34]
>>>
```

When you have finished, type `Ctrl-D` to close the REPL's input stream.

Embed the interpreter in your Go program:

```go
import "github.com/canonical/starlark"

// Execute Starlark program in a file.
thread := &starlark.Thread{Name: "my thread"}
globals, err := starlark.ExecFile(thread, "fibonacci.star", nil, nil)
if err != nil { ... }

// Retrieve a module global.
fibonacci := globals["fibonacci"]

// Call Starlark function from Go.
v, err := starlark.Call(thread, fibonacci, starlark.Tuple{starlark.MakeInt(10)}, nil)
if err != nil { ... }
fmt.Printf("fibonacci(10) = %v\n", v) // fibonacci(10) = [0, 1, 1, 2, 3, 5, 8, 13, 21, 34]
```

See [starlark/example_test.go](starlark/example_test.go) for more examples.

### Contributing

We welcome submissions but please let us know what you're working on
if you want to change or add to the Starlark repository.

Before undertaking to write something new for the Starlark project,
please file an issue or claim an existing issue.
All significant changes to the language or to the interpreter's Go
API must be discussed before they can be accepted.
This gives all participants a chance to validate the design and to
avoid duplication of effort.

Despite some differences, the Go implementation of Starlark strives to
match the behavior of [the Java implementation](https://github.com/bazelbuild/bazel)
used by Bazel and maintained by the Bazel team.
For that reason, proposals to change the language itself should
generally be directed to [the Starlark site](
https://github.com/bazelbuild/starlark/), not to the maintainers of this
project.
Only once there is consensus that a language change is desirable may
its Go implementation proceed.

We use GitHub pull requests for contributions.

<!-- TODO(kcza): update to our CLA... and theirs too? -->
Please complete Google's contributor license agreement (CLA) before
sending your first change to the project.  If you are the copyright
holder, you will need to agree to the
[individual contributor license agreement](https://cla.developers.google.com/about/google-individual),
which can be completed online.
If your organization is the copyright holder, the organization will
need to agree to the [corporate contributor license agreement](https://cla.developers.google.com/about/google-corporate).
If the copyright holder for your contribution has already completed
the agreement in connection with another Google open source project,
it does not need to be completed again.

### Stability

We reserve the right to make breaking language and API changes at this
stage in the project, although we will endeavor to keep them to a minimum.
Once the Bazel team has finalized the version 1 language specification,
we will be more rigorous with interface stability.

We aim to support the most recent four (go1.x) releases of the Go
toolchain. For example, if the latest release is go1.20, we support it
along with go1.19, go1.18, and go1.17, but not go1.16.

### Credits

Starlark was designed and implemented in Java by
Jon Brandvein,
Alan Donovan,
Laurent Le Brun,
Dmitry Lomov,
Vladimir Moskva,
François-René Rideau,
Gergely Svigruha, and
Florian Weikert,
standing on the shoulders of the Python community.
The Go implementation was written by Alan Donovan and Jay Conrod;
its scanner was derived from one written by Russ Cox.

### Legal

Starlark in Go is Copyright (c) 2018 The Bazel Authors.
All rights reserved.

It is provided under a 3-clause BSD license:
[LICENSE](https://github.com/canonical/starlark/blob/main/LICENSE).

Starlark in Go is not an official Google product.
