# Cooperative Resource Management

Resource management is a crucial aspect in every modern industry, and the IT sector is no exception. The types of resources typically considered include:

 - Memory
 - CPU cycles
 - I/O (Network, Disk, etc.)
 - Time [^1]

While it is often acceptable to allow their usage to grow without bounds, especially when resources are abundant, there are instances where more stringent control becomes necessary.

The only entity with complete control over most of these resources is the operating system (OS). However, the OS typically lacks the context to apply fine-grained policies when an application reaches its imposed limits. Exceeding a limit usually results in the termination of the entire application [^2]. Additionally, implementing solutions at the OS level often requires platform-specific implementations, making it challenging to abstract in higher-level and cross-platform languages.

Some languages enable control over memory allocation, either through custom allocators [^3] or facilities like *Arenas* [^4]. In the former, a custom allocator fetches memory from the environment, allowing for usage limitations. In the latter, the maximum amount of memory is acquired upfront, and all allocations come from that predefined region.

Given the limitations of these solutions, a language-independent and cross-platform approach is preferable.

## Best-Effort Cooperation

While it may be challenging to guarantee safety in resource management generally, purposefully crafted software can achieve a similar effect through *cooperation*.

This parallel is akin to the relationship between *fibers*/*coroutines* and *threads*:

- *Threads* are a general-purpose facility requiring an OS (and hardware support!) to preempt a running routine and share resources.
- *Fibers*/*coroutines* necessitate the running fiber to explicitly return control to the caller.

Certainly, *fibers* (or *coroutines*) are not immune to malicious software, which can create fairness issues and prevent other routines from running. Nevertheless, their execution is fast, and crucially, their implementation does not rely on any platform-specific features, making them a valuable abstraction for any language [^5].

The *cooperation* concept behind them can be extended to virtually any resource.

### Cancellation (Time)

Cooperation over a *cancellation*[^6] aspect has been explored by many languages such as [C++](https://en.cppreference.com/w/cpp/header/stop_token), [Rust](https://docs.rs/stop-token/latest/stop_token/), [C#](https://learn.microsoft.com/en-us/dotnet/standard/threading/cancellation-in-managed-threads), [Go](https://pkg.go.dev/context) and [JavaScript](https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal) (and probably more).

In this case, each routine is responsible for managing the cancellation event or waiting for cancellation to occur.

While nothing prevents a malicious (or poorly written) routine from ignoring the token altogether, there is no firm guarantee that an execution thread will actually stop once the token is canceled (the *cooperation* aspect) or meet any time deadline for the cancellation (the *best-effort* aspect). However, in practice, this works in most cases.

[^1]: While time is typically a derivative resource (i.e. a function of memory, I/O and CPU cycles), it primarily influences the user's perception, making it important in its own right.

[^2]: Although it is possible to react before termination (e.g. cgroups) or isolate the execution of each part of the application in a different process for finer granularity, this approach is more resource-intensive and necessitates inter-process communication (IPC) and/or per-platform solutions.

[^3]: [C++](https://en.cppreference.com/w/cpp/named_req/Allocator) and [Rust](https://doc.rust-lang.org/std/alloc/trait.Allocator.html), for example, support custom allocators throughout the standard library.

[^4]: [Java](https://docs.oracle.com/en/java/javase/20/docs/api/java.base/java/lang/foreign/Arena.html) is developing arenas. C has been using arenas for performance in various projects (e.g., [Apache Httpd](https://apr.apache.org/docs/apr/1.5/group__apr__allocator.html)). Go doesn't have them, but there is [some discussion](https://github.com/golang/go/issues/51317) about them.

[^5]: While it is true that fibers or coroutines *in general* do not depend on the platform, some implementations still rely on details to ease the job of the compiler or overcome certain compiler limitations (like the lack of coroutines).

[^6]: Since a cancellation token (or abort signal) can be given a timeout, cooperative (or best-effort) cancellation manages *time* as well as some other sources of cancellation.
