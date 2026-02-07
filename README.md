# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/lfq)](https://goreportcard.com/report/github.com/hayabusa-cloud/lfq)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Languages:** English | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | [Español](README.es.md) | [Français](README.fr.md)

Lock-free and wait-free FIFO queue implementations for Go.

## Overview

Package `lfq` provides bounded FIFO queues optimized for different producer/consumer patterns. Each variant uses the suitable algorithm for its access pattern.

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

**Requirements:** Go 1.25+

### Compiler Requirement

For better performance, compile with the [intrinsics-optimized Go compiler](https://github.com/hayabusa-cloud/go):

```bash
# Using Makefile (recommended)
make install-compiler   # Download pre-built release (~30 seconds)
make build              # Build with intrinsics compiler
make test               # Test with intrinsics compiler

# Or build compiler from source (bleeding-edge)
make install-compiler-source
```

Manual installation:

```bash
# Pre-built release (recommended)
URL=$(curl -fsSL https://api.github.com/repos/hayabusa-cloud/go/releases/latest | grep 'browser_download_url.*linux-amd64\.tar\.gz"' | cut -d'"' -f4)
curl -fsSL "$URL" | tar -xz -C ~/sdk
mv ~/sdk/go ~/sdk/go-atomix

# Use for building lfq-dependent code
GOROOT=~/sdk/go-atomix ~/sdk/go-atomix/bin/go build ./...
```

The intrinsics compiler inlines `atomix` operations with proper memory ordering. The standard Go compiler works for basic testing but may exhibit issues under high contention.

## Queue Types

| Type | Pattern | Progress Guarantee | Use Case |
|------|---------|-------------------|----------|
| **SPSC** | Single-Producer Single-Consumer | Wait-free | Pipeline stages, channels |
| **MPSC** | Multi-Producer Single-Consumer | Lock-free | Event aggregation, logging |
| **SPMC** | Single-Producer Multi-Consumer | Lock-free | Work distribution |
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

By default, multi-access queues use FAA (Fetch-And-Add) based algorithms derived from SCQ (Scalable Circular Queue). FAA blindly increments position counters, requiring 2n physical slots for capacity n, but scales better under high contention than CAS-based alternatives.

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

Indirect and Ptr queue variants (non-SPSC, non-Compact) pack sequence number and value into a single 128-bit atomic. This reduces cache line contention and improves throughput under high concurrency.

```go
// Indirect - single 128-bit atomic per operation
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

// Pointer - zero-copy
q := lfq.NewMPMCPtr(1024)
q.Enqueue(unsafe.Pointer(obj))
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

SPSC variants already use n slots (Lamport ring buffer) and ignore Compact(). For Indirect queues with Compact(), values are limited to 63 bits.

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

### Buffer Pool

```go
const poolSize = 1024
const bufSize = 4096

// Pre-allocate buffers
pool := make([][]byte, poolSize)
for i := range pool {
    pool[i] = make([]byte, bufSize)
}

// Free list tracks available indices
freeList := lfq.NewSPSCIndirect(poolSize)
for i := range poolSize {
    freeList.Enqueue(uintptr(i))
}

// Allocate
func Alloc() ([]byte, uintptr, bool) {
    idx, err := freeList.Dequeue()
    if err != nil {
        return nil, 0, false
    }
    return pool[idx], idx, true
}

// Free
func Free(idx uintptr) {
    freeList.Enqueue(idx)
}
```

### Event Aggregation

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
        for reading := range s.Readings() {
            ev := Event{
                Source:    s.Name(),
                Timestamp: time.Now(),
                Data:      reading,
            }
            events.Enqueue(&ev)
        }
    }(sensor)
}

// Single aggregator (single consumer)
go func() {
    for {
        ev, err := events.Dequeue()
        if err == nil {
            aggregate(*ev)
        }
    }
}()
```

### Backpressure Handling

```go
// With retry and yield
func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxRetries int) bool {
	ba := iox.Backoff{}
    for i := range maxRetries {
        if q.Enqueue(&item) == nil {
            return true
        }
        ba.Wait() // Yield to let consumers drain
    }
    return false // Apply backpressure to caller
}

```

### Graceful Shutdown

FAA-based queues (MPMC, SPMC, MPSC) include a threshold mechanism to prevent livelock. For graceful shutdown where producers finish before consumers, use the `Drainer` interface:

```go
// Producer goroutines finish
prodWg.Wait()

// Signal no more enqueues will occur
if d, ok := q.(lfq.Drainer); ok {
    d.Drain()
}

// Consumers can now drain all remaining items
// without threshold blocking
for {
    item, err := q.Dequeue()
    if err != nil {
        break // Queue is empty
    }
    process(item)
}
```

`Drain()` is a hint — the caller must ensure no further `Enqueue()` calls will be made. SPSC queues do not implement `Drainer` as they have no threshold mechanism; the type assertion naturally handles this case.

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

Go's race detector is not designed for lock-free algorithm verification. It tracks explicit sync primitives (mutex, channels) but cannot observe happens-before relationships from atomic memory orderings.

Tests use two protection mechanisms:
- Build tag `//go:build !race` excludes example files from race testing
- Runtime check `if lfq.RaceEnabled { t.Skip() }` skips concurrent tests in `lockfree_test.go`

Run `go test -race ./...` for race-safe tests, or `go test ./...` for all tests.

## Dependencies

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Semantic errors (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — Atomic primitives with explicit memory ordering
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — Spin primitives

## Platform Support

| Platform | Status |
|----------|--------|
| linux/amd64 | Primary |
| linux/arm64 | Supported |
| linux/riscv64 | Supported |
| linux/loong64 | Supported |
| darwin/amd64, darwin/arm64 | Supported |
| freebsd/amd64, freebsd/arm64 | Supported |

## References

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *arXiv*, arXiv:1908.04511. https://arxiv.org/abs/1908.04511.
- Lamport, L. (1974). A New Solution of Dijkstra's Concurrent Programming Problem. *Communications of the ACM*, 17(8), 453–455.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## License

MIT — see [LICENSE](./LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
