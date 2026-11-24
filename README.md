# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Benchmarks](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Languages:** English | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | [Français](README.fr.md)

Lock-free and wait-free FIFO queue implementations for Go.

## Overview

Package `lfq` provides lock-free and wait-free bounded FIFO queues optimized for different producer/consumer patterns (SPSC, MPSC, SPMC, MPMC), ensuring predictable, zero-allocation scaling under high contention.

### Theory and Background

`lfq` builds on foundational concurrency research to overcome the limitations of traditional mutexes and CAS (Compare-And-Swap) loops:

- **Lamport Ring Buffer (1977)**: Powers our Single-Producer/Single-Consumer (SPSC) paths, offering strictly $O(1)$ wait-free latency without expensive atomic read-modify-write instructions.
- **Scalable Circular Queue (SCQ, 2019)**. Ruslan Nikolaev’s SCQ is the basis for the generic FAA queues.

### The `iox` Non-Blocking I/O Stacks

When a queue operation returns `ErrWouldBlock`, an event loop can service other work and retry later. A synchronous caller can wait with `iox.Backoff`.

```go
// Direct constructor (recommended for most cases)
q := lfq.NewSPSC[Event](1024)

// Builder API - auto-selects algorithm based on constraints
q := lfq.Build[Event](lfq.New(1024).SingleProducer().SingleConsumer())  // → SPSC
q := lfq.Build[Event](lfq.New(1024).SingleConsumer())                   // → MPSC
q := lfq.Build[Event](lfq.New(1024).SingleProducer())                   // → SPMC
q := lfq.Build[Event](lfq.New(1024))                                    // → MPMC
```

## Installation

```bash
go get code.hybscloud.com/lfq
```

**Requirements:** Go 1.27+

### Compiler Requirement

Use the Atomix Go compiler for production builds and benchmarks. [Atomix Go](https://github.com/hayabusa-cloud/go)

```bash
# Using Makefile (recommended)
make install-compiler   # Linux amd64/arm64 or macOS arm64
make build              # Build with intrinsics compiler
make test               # Test with intrinsics compiler

# Or build compiler from source
make install-compiler-source
```

Both install targets default to the latest published Go 1.27.x Atomix compiler. Export `COMPILER_TAG` to select a specific release. Release discovery uses `jq` and can authenticate to GitHub with `GH_TOKEN`. Source builds use the latest stock Go 1.27.x on `PATH`.

For manual installation, download the archive for your platform and verify it against `SHA256SUMS` from the same release. Place the extracted `go` directory at `$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}`. [Compiler releases](https://github.com/hayabusa-cloud/go/releases)

```bash
# Build an application that uses lfq
compiler="$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}"
GOTOOLCHAIN=local GOROOT="$compiler" "$compiler/bin/go" build ./...
```

The Atomix compiler inlines `atomix` operations. CI runs correctness tests with standard Go.

## Benchmarks

CI runs `make bench-ci` on native Linux amd64 and arm64 with the Atomix compiler and `GOMAXPROCS=2`. Each Actions summary shows the median of ten samples for session handoff and retry, an enqueue/dequeue pair on each of the four queue types, and SPMC with concurrent consumers. The workflow artifacts contain the raw samples.

`Handoff` transfers one value between two goroutines through the four-slot SPSC data queue used by `sess` or a buffered Go channel of the same capacity. Both use `iox.Backoff` on a full or empty queue. `ns/op` includes that wait time, and `waits/op` counts waits on both sides per delivered value.

`FullRetry` measures a failed enqueue on a full queue, one dequeue, and a successful retry in a single goroutine. Run `make bench-session` for the session cases or `make bench` for the full suite.

## Queue Types

| Type | Pattern | Progress Guarantee | Use Case |
|------|---------|-------------------|----------|
| **SPSC** | Single-Producer Single-Consumer | Wait-free | Pipeline stages, channels |
| **MPSC** | Multi-Producer Single-Consumer | Lock-free (wait-free dequeue) | Event aggregation, logging |
| **SPMC** | Single-Producer Multi-Consumer | Lock-free (wait-free enqueue) | Work distribution |
| **MPMC** | Multi-Producer Multi-Consumer | Lock-free | General purpose |

### Progress Guarantees

- **Wait-free**: Every operation completes in bounded steps
- **Lock-free**: System-wide progress guaranteed; at least one thread makes progress

## Algorithms

### SPSC: Lamport Ring Buffer

Classic bounded buffer with cached index optimization. 

```go
q := lfq.NewSPSC[int](1024)

// Producer
q.Enqueue(&value)  // Wait-free O(1)

// Consumer
elem, err := q.Dequeue()  // Wait-free O(1)
```

### MPSC/SPMC/MPMC: FAA-Based (Default)

Default multi-access queues use fetch-and-add (FAA) to claim positions in a ring of `2n` slots. In `MPMCIndirect` and `MPMCPtr`, workers publish and remove values at consecutive slot positions. Another worker can use the next position if a claimant pauses.

- **Trade-off**: Requires `2n` physical slots for nominal capacity `n`.
- **Transient Capacity**: With `P` concurrent producers, up to `P-1` additional items may be transiently enqueued beyond `Cap()` before backpressure applies.

```go
// Multiple producers, single consumer
q := lfq.NewMPSC[Event](1024)  // FAA producers, wait-free dequeue

// Single producer, multiple consumers
q := lfq.NewSPMC[Task](1024)   // Wait-free enqueue, FAA consumers

// Multiple producers and consumers
q := lfq.NewMPMC[*Request](4096)  // FAA-based SCQ algorithm
```

Cycle-based slot validation provides ABA safety without epoch counters or hazard pointers.

### Indirect/Ptr Variants: 128-bit Atomic Operations

Indirect and Ptr queue variants (all non-SPSC variants except Compact Indirect) pack sequence number and payload into a single 128-bit atomic. This reduces cache line contention and improves throughput under high concurrency.

For Ptr variants that encode pointers in 128-bit slots (FAA and PtrSeq), keep a typed reference to each Go object at a stable address until its consumer takes ownership.

```go
// Indirect stores the sequence and value in one atomic pair.
q := lfq.NewMPMCIndirect(4096)

// Ptr - same optimization for unsafe.Pointer
q := lfq.NewMPMCPtr(4096)
```

## Builder API

Automatic algorithm selection based on constraints:

```go
// SPSC - both constraints → Lamport ring
q := lfq.Build[T](lfq.New(1024).SingleProducer().SingleConsumer())

// MPSC - single consumer only
q := lfq.Build[T](lfq.New(1024).SingleConsumer())

// SPMC - single producer only
q := lfq.Build[T](lfq.New(1024).SingleProducer())

// MPMC - no constraints (default)
q := lfq.Build[T](lfq.New(1024))
```

## Variants

Each queue type has three variants:

| Variant | Element Type | Use Case |
|---------|--------------|----------|
| Generic | `[T any]` | Type-safe, general purpose |
| Indirect | `uintptr` | Index-based pools, handles |
| Ptr | `unsafe.Pointer` | Zero-copy pointer passing |

```go
// Generic
q := lfq.NewMPMC[MyStruct](1024)

// Indirect - for pool indices
q := lfq.NewMPMCIndirect(1024)
q.Enqueue(uintptr(poolIndex))

// Pointer into a package-level message array
q := lfq.NewMPMCPtr(len(messages))
q.Enqueue(unsafe.Pointer(&messages[i]))
```

### Compact Mode

Compact() selects CAS-based algorithms that use n physical slots (vs 2n for FAA-based default). Use when memory efficiency is more important than contention scalability:

```go
// Compact mode - CAS-based, n slots
q := lfq.New(4096).Compact().BuildIndirect()
```

| Mode | Algorithm | Physical Slots | Use When |
|------|-----------|----------------|----------|
| Default | FAA-based | 2n | High contention, scalability |
| Compact | CAS-based | n | Memory constrained |

SPSC variants already use n slots (Lamport ring buffer) and ignore `Compact()`. For other topologies, `Compact().BuildIndirect()` accepts 63-bit values. `SPMCCompactIndirect` and `MPMCCompactIndirect` also require values to stay distinct while concurrent consumers can observe them.

For duplicate `uintptr` values with multiple consumers, use `NewSPMCIndirectSeq` with one producer or `NewMPMCIndirect` with multiple producers.

## Operations

| Operation | Returns | Description |
|-----------|---------|-------------|
| `Enqueue(elem)` | `error` | Add element; returns `ErrWouldBlock` if full |
| `Dequeue()` | `(T, error)` | Remove element; returns `ErrWouldBlock` if empty |
| `Cap()` | `int` | Queue capacity |

### Error Handling

```go
err := q.Enqueue(&item)
if lfq.IsWouldBlock(err) {
    // Queue is full - backpressure or retry
}

elem, err := q.Dequeue()
if lfq.IsWouldBlock(err) {
    // Queue is empty - wait or poll
}
```

## Usage Patterns

The following patterns demonstrate how `lfq` can be integrated into concurrent systems. By avoiding allocations in the hot path and utilizing appropriate queue variants, you can achieve substantial latency reductions.

### Buffer Pool

One goroutine borrows buffers from the pool and one returns them. Each successful `alloc` leaves room in the free list for a return. Call `free` exactly once, after the buffer’s last use.

```go
const poolSize = 1024
const bufSize = 4096

pool := make([][]byte, poolSize)
freeList := lfq.NewSPSCIndirect(len(pool))
for i := range pool {
	pool[i] = make([]byte, bufSize)
	freeList.Enqueue(uintptr(i))
}

alloc := func() ([]byte, uintptr, bool) {
	idx, err := freeList.Dequeue()
	if err != nil {
		return nil, 0, false
	}
	return pool[idx], idx, true
}
free := func(idx uintptr) {
	freeList.Enqueue(idx)
}

if buf, idx, ok := alloc(); ok {
	copy(buf, "hello")
	free(idx)
}
```

### Event Aggregation

The aggregator runs for the lifetime of the service. Each producer keeps its current reading until the queue accepts it.

```go
type Event struct {
	Source    string
	Timestamp time.Time
	Data      any
}

// Multiple sources → Single processor
events := lfq.NewMPSC[Event](8192)

// Event sources (multiple producers)
for sensor := range slices.Values(sensors) {
	go func(s Sensor) {
		var backoff iox.Backoff
		for reading := range s.Readings() {
			ev := Event{
				Source:    s.Name(),
				Timestamp: time.Now(),
				Data:      reading,
			}
			for events.Enqueue(&ev) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
	}(sensor)
}

// Single aggregator (single consumer)
go func() {
	var backoff iox.Backoff
	for {
		ev, err := events.Dequeue()
		if err != nil {
			backoff.Wait()
			continue
		}
		backoff.Reset()
		aggregate(ev)
	}
}()
```

### Backpressure Handling

The caller-defined helper `func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxAttempts int) bool` counts the first attempt toward its limit. A zero limit returns `false` immediately.

```go
var backoff iox.Backoff
for attempt := range maxAttempts {
	if attempt > 0 {
		backoff.Wait()
	}
	if q.Enqueue(&item) == nil {
		return true
	}
}
return false
```

### Graceful Shutdown

The generic FAA MPMC and FAA SPMC queues use a dequeue threshold. `Drain` disables that threshold after production ends. `MPMCIndirect` and `MPMCPtr` can always drain their published values.

```go
// Producer goroutines finish
prodWg.Wait()

// Signal no more enqueues will occur
if d, ok := q.(lfq.Drainer); ok {
    d.Drain()
}

// Dequeue until the queue reports empty.
for {
    item, err := q.Dequeue()
    if err != nil {
        break
    }
    process(item)
}
```

`Drain()` is a hint — the caller must ensure no further `Enqueue()` calls will be made. For queues with a dequeue threshold, `Drain()` avoids threshold-based early exit in `Dequeue`. For MPSC, `Drain()` is a shutdown signal only. SPSC queues do not implement `Drainer`; the type assertion naturally handles this case.

## When to Use Which Queue

```
┌─────────────────────────────────────────────────────────────────┐
│                    How many producers?                          │
│                                                                 │
│      ┌──────────────────┐          ┌────────────────────┐      │
│      │    One (SPSC/     │          │   Multiple (MPMC/  │      │
│      │    SPMC)          │          │   MPSC)            │      │
│      └────────┬─────────┘          └─────────┬──────────┘      │
│               │                               │                 │
│               ▼                               ▼                 │
│   ┌──────────────────┐              ┌──────────────────┐       │
│   │ One consumer?    │              │ One consumer?    │       │
│   └────────┬─────────┘              └────────┬─────────┘       │
│    Yes     │     No                  Yes     │     No          │
│     │      │      │                   │      │      │          │
│     ▼      │      ▼                   ▼      │      ▼          │
│   SPSC     │    SPMC                MPSC     │    MPMC         │
│            │                                 │                  │
└────────────┴─────────────────────────────────┴─────────────────┘

Variant Selection:
• Generic [T]     → Type-safe, copying semantics
• Indirect        → Pool indices, buffer offsets (uintptr)
• Ptr             → Zero-copy object passing (unsafe.Pointer)
```

### Capacity

Capacity rounds up to the next power of 2:

```go
q := lfq.NewMPMC[int](3)     // Actual capacity: 4
q := lfq.NewMPMC[int](4)     // Actual capacity: 4
q := lfq.NewMPMC[int](1000)  // Actual capacity: 1024
q := lfq.NewMPMC[int](1024)  // Actual capacity: 1024
```

Minimum capacity is `2`. Constructors panic if `capacity < 2`.

## Memory Layout

All queues use cache-line padding (64 bytes) to prevent false sharing:

```go
type MPMC[T any] struct {
    _        [64]byte      // Padding
    tail     atomix.Uint64 // Producer index
    _        [64]byte      // Padding
    head     atomix.Uint64 // Consumer index
    _        [64]byte      // Padding
    buffer   []slot[T]
    // ...
}
```

## Race Detection

Go’s race detector cannot observe all synchronization in `atomix` assembly. Run `go test -race ./...` for the instrumented tests. Run `go test ./...` to include the concurrent tests and examples skipped in race builds.

## Dependencies

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Semantic errors (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — Atomic primitives with explicit memory ordering
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — Spin primitives

## Platform Support

| Platform | Status |
|----------|--------|
| linux/amd64, linux/arm64 (LSE2) | Production on Linux 6.18+ |
| linux/riscv64, linux/loong64 | Cross-built in CI |
| darwin/amd64, darwin/arm64 | Development. CI tests darwin/arm64. |
| freebsd/amd64, freebsd/arm64 | Development |

## References

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *DISC 2019 (LIPIcs)*. https://doi.org/10.4230/LIPIcs.DISC.2019.28. Preprint: https://arxiv.org/abs/1908.04511.
- Lamport, L. (1977). Proving the Correctness of Multiprocess Programs. *IEEE Transactions on Software Engineering*, 3(2), 125–143.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## License

MIT — see [LICENSE](./LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
