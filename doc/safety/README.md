# Cooperative Resource Management

Resource management is a crucial aspect in every modern industry, and the IT sector is no exception. The types of resources typically considered include:

 - Time[^1]
 - Memory
 - CPU usage
 - I/O (Network, Disk, etc.)

While it is often acceptable to allow resource usage to grow without bounds, there are situations where more stringent control is necessary.

## Best-Effort Cooperation

The only thing with complete control over most of these resources is the operating system. However, the OS typically lacks the context to apply fine-grained policies when an application reaches its imposed limits. Exceeding a limit usually results in the termination of the entire application[^2]. Additionally, implementing solutions at the OS level often requires platform-specific implementations, impeding high-level, cross-platform abstractions.

The next best thing would be to use language-specific features to control resources. Although it's possible in some languages to control memory allocation, either through custom allocators[^3] or facilities like *Arenas*[^4], it's typically impossible to finely control other resources such as time or CPU usage.

However, often precise control is not necessary and some slight resource overuse is acceptable. For instance, it is usually ok to stop a routine within a small amount of time after the deadline or to consume few more bytes from memory. This makes the limit approximate, but nevertheless useful. In this document, solutions enforcing limits in this way will be referred to as *best-effort.*

Higher level abstractions cannot rely on lower level tools such as the OS or the hardware, hence code must be added to explicitly account for them. This means that if a routine does not add the necessary logic, the system will misbehave. On the countrary, if all (or at least most) routines *cooperate* together, the system will be able to limit resources without any external aid.

A well known example of these two concepts are *fibers*, which are an implementation of *cooperative multitasking*. Whilst *threads* are a general-purpose facility which requires an OS and hardware support to preempt a running routine, fibers must explicitly return control to the scheduler. In line with the usual cooperative bounding perils, fibers are not immune to maliciously or poorly written software, which can create fairness issues and prevent other routines from running. Nevertheless, many systems are built using them as their execution is fast and crucially their implementation does not rely on any platform-specific features, making them a valuable abstraction for many languages[^5]. Moreover, the explicit control fibers have over when to switch is crucial - it is not efficient to allow control to be yielded at every instruction. In this case, the trade-off is up to the implementer/designer who has an overview of the goals of the system. This trade-off between precision and simplicity/speed is what makes this cooperative scheme also *best-effort*.

This idea of best-effort cooperation can be extended to manage all resource types mentioned at the beginning of this document. An overview of how it may be achieved them follows.

### Time

Cooperation over a *cancellation token*[^6] aspect is used by many languages including [C++](https://en.cppreference.com/w/cpp/header/stop_token), [Rust](https://docs.rs/stop-token/latest/stop_token/), [C#](https://learn.microsoft.com/en-us/dotnet/standard/threading/cancellation-in-managed-threads), [Go](https://pkg.go.dev/context) and [JavaScript](https://developer.mozilla.org/en-US/docs/Web/API/AbortSignal).

In this case, each routine is responsible for checking for and reacting to a cancellation event. Of course, nothing stops a routine from neglecting the token (*cooperative*) and there is no firm guarantee that an execution thread will actually stop once the token is canceled or meet any time deadline for the cancellation (*best-effort*), in practice, this works well in most cases.

### Memory

Memory management consists of two primitive operations: allocating and releasing. The primary objective of memory management is to ensure that $M_{used} \leq M_{limit}$.

This implies that it is possible to cooperatively limit the amount of memory used by an execution thread[^7] by monitoring the memory amount before performing an allocation and comparing the total to a limit. If there is insufficient memory available, the routine should abstain from the allocation. This can be referred to as the *cooperation* aspect for memory.

This simple concept can be enhanced by considering memory release as well. Memory is one of the few resources that can both *grow* and *shrink*.

While in (semi-)manually managed memory languages like C, C++, and Rust, it is clear when and how to remove the allocation from the amount used, as both the lifetime and the size of objects are explicit[^8], it still poses questions around *ownership*. Specifically:

 - How to constrain memory for objects shared among two or more separate constrained execution contexts.
 - How to account for memory of objects that should outlive a constrained region of code.
 - How to account for memory allocated outside but used inside a constrained region.

On the other hand, automatic memory management (like Garbage Collection) makes it almost impossible to reliably account for memory release. Moreover, higher-level languages sometimes make it difficult to precisely measure the size of an object tree.

In all these cases, a conservative approach can be followed by *estimating the amount of memory used* such that: $M_{estimated} \geq M_{used}$. Consequently, if $M_{estimated} \leq M_{limit}$, then $M_{used} \leq M_{limit}$ holds. This can be referred to as the *best-effort* aspect for memory.

This simplification can be beneficial when it is challenging to know the size of an object and when understanding the lifetime of an object is difficult (or impossible). In the former case, it is usually possible to *overestimate*. In the latter case, it is sufficient to never release memory[^9].

### CPU Cycles

CPU cycles are a contentious resource. In theory, it is possible to measure the amount of cycles precisely using hardware/OS features. Moreover, once computational power is utilized, it cannot be reclaimed, making the usage of this resource inherently monotonic.

Despite this, several factors complicate the measurement and limitation of CPU cycles:

- In a concurrent environment, the total count of executed instructions is not deterministic[^10].
- The cost of an instruction is generally not a fixed measure on modern processors[^11].
- The cost of an instruction depends on *the processor*, not the instruction set.
- The execution cost may change depending on the cache state of the processor.
- Unpredictable interactions between user-level and kernel components make measurements challenging.
- The same routine may switch implementation depending on the available instruction set or input.
- And more.

Furthermore, many measurements are OS-/platform-specific and challenging to map in a high-level language.

There is also some overlap between [Cancellation](#Cancellation-Time) and CPU cycles. However, it is useful to distinguish between the two cases.
For example:

- In the case of high system load, *wall clock time* continues to progress even if the routine doesn't get any chance to execute.
- In the case of a blocking I/O call, the same applies.

In all these cases, CPU cycles will not be noticeably affected.

Delving into the reasons for limiting CPU cycles, the rationale can be summarized: it's acceptable for a code region to take more *wall clock time* (e.g., during I/O), but it's unacceptable for this region to consume all computational resources during that time[^12]. This line of thinking not only renders this limit useful but also suggests a way to simplify its management. Having a rough measure of the execution cost of each routine (referred to as *steps* from now on) ensures that the function doesn't monopolize all computational resources. This cost doesn't need to be consistent among different routines or with real CPU cycles; it is sufficient for it to be somehow proportional to the number of operations performed. Moreover, the same reasoning about overestimating carried out in [Memory](#Memory) can be applied here. This concept is the *best-effort* aspect for CPU cycles.

Similar to memory, the *cooperation* aspect here involves each routine counting its own steps and verifying that there are enough left to continue the computation.

### I/O

While constraining I/O in a manner similar to Memory and CPU is certainly possible, the diverse nature of I/O doesn't lend itself to a shared framework and needs to be discussed on a per-resource type basis. For instance, disk usage could be constrained from the perspective of size (similar to memory), but it should also be constrained from the point of view of folders. In contrast, network usage could be constrained in total size or in bits per second (throughput), depending on the primary target.

It is likely that the dualism of *best-effort*/*cooperative* can be successfully applied in I/O management as well.



[^1]: Whilst time is a derived resource (i.e. a function of memory, I/O and CPU cycles), it primarily influences the user's perception, making it important in its own right.

[^2]: Although it is possible to react before termination (e.g. using *cgroups*) or to isolate the execution of each part of the application in a different process for finer granularity, these approaches are more resource-intensive and necessitate IPC and/or per-platform solutions.

[^3]: [C++](https://en.cppreference.com/w/cpp/named_req/Allocator) for example, support custom allocators throughout the standard library.

[^4]: C has been using arenas for performance in various projects for decades (e.g., [Apache Httpd](https://apr.apache.org/docs/apr/1.5/group__apr__allocator.html)). [Rust](https://crates.io/crates/bumpalo) has many crates for arenas. [Java](https://docs.oracle.com/en/java/javase/20/docs/api/java.base/java/lang/foreign/Arena.html) is developing arenas. Go doesn't have them, but there is [some discussion](https://github.com/golang/go/issues/51317) about them.

[^5]: While it is true that fibers *in general* do not depend on the platform, some implementations still rely on details to ease the job of the compiler or to overcome certain limitations, such as a lack of coroutines.

[^6]: Since a cancellation token (or abort signal) can be given a timeout, cooperative (or best-effort) cancellation manages *time* as well as some other sources of cancellation.

[^7]: In this case, the term *execution thread* refers to the logical execution of a group of routines, not the OS thread facility.

[^8]: the determinism of the release of memory can be lost if ref-counted resources are used and is shared among more than one concurrent execution thread. This makes it difficult even for those language to properly limit memory without occuring in non-deterministic behaviors.

[^9]: While never counting memory releases may seem problematic, in practice, it is significant primarily for long-running routines. Usually, Arenas follow the same approach, never releasing memory during their lifetime but doing so in a single operation when the Arena itself is discarded.

[^10]: Contention and patterns like [spinlocks](https://en.wikipedia.org/wiki/Spinlock) and [optimistic concurrency control](https://en.wikipedia.org/wiki/Optimistic_concurrency_control) are clear examples of why this happens.

[^11]: See [Speculative execution](https://en.wikipedia.org/wiki/Speculative_execution) and [Instruction pipelining](https://en.wikipedia.org/wiki/Instruction_pipelining) for quick examples.

[^12]: Imagine allowing 1 minute of execution, so that a network/IO request can be properly executed. An unconstrained logic might use 1 minute of CPU time, without performing any IO. This might easily consume too much computational power.
