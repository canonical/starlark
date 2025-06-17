# Starlark in Go

[![Go Tests](https://github.com/canonical/starlark/actions/workflows/tests.yml/badge.svg)](https://github.com/canonical/starlark/actions/workflows/tests.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/canonical/starlark.svg)](https://pkg.go.dev/github.com/canonical/starlark)

This is a patch on Google's _Starlark in Go_ project, maintained by Canonical, which adds safety constraints to the language.
It is developed to be a mostly-drop-in replacement for upstream Starlark and as such has small number of trivial breaking changes.

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

- [Safety constraints](doc/safety.md):
    - `MemSafe` – scripts respect a given memory budget
    - `CPUSafe` – scripts respect a CPU-time budget
    - `TimeSafe` – scripts respect deadlines and cancellation
    - `IOSafe` – scripts respect access to the host system
- A new safety-testing framework, [`startest`](https://pkg.go.dev/github.com/canonical/starlark/startest)
- [Safe helpers for common operations](doc/safety.md#common-patterns)
- Memory-usage estimation for arbitrary values
- [`Context`](https://pkg.go.dev/context) integration with the execution `Thread`
- Starlark-wide overflow protection
- Fallable iterators with the new `Err()` method
- Starlark recursion now has a hard limit
- Threads cannot be uncancelled
- A small number of trivial breaking changes

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
$ go install github.com/canonical/starlark/cmd/starlark@latest
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
import "github.com/canonical/starlark/starlark"

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

Please complete [Canonical's contributor license agreement (CLA)](https://ubuntu.com/legal/contributors) before
sending your first change to the project.

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

### License

Canonical's Starlark patch is provided under the same 3-clause BSD license as its upstream:
[LICENSE](https://github.com/canonical/starlark/blob/main/LICENSE).
