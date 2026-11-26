# lfq - Copilot Instructions

Lock-free FIFO queues.

## Compiler Requirement

Use Atomix Go for local builds, production builds, and benchmarks. `make install-compiler` selects the latest Atomix Go 1.27.x release by default. [Compiler setup](../../README.md#compiler-requirement)

Correctness CI runs the race detector and coverage with standard Go. [CI workflow](../workflows/ci.yml)

## Quick Reference

| Queue | Progress | Default | Compact |
|-------|----------|---------------|---------------|
| SPSC | Wait-free | n slots, cached ring | Same |
| MPSC/SPMC/MPMC | Lock-free | 2n slots, FAA | n slots, CAS |

## Review Focus

Review each queue under its documented producer and consumer contract.

- Memory ordering (acquire/release pairs)
- Slot count matches algorithm (see Quick Reference)
- Contended loops inside queue operations use `spin.Wait`.

## Pitfalls

- Mixing FAA slot math (2n) with CAS slot math (n)
- Waiting for the other endpoint inside an SPSC operation

## Error Handling

`ErrWouldBlock` means the call transferred no item. A producer may still be publishing the next item, or a consumer may still be releasing the next slot. An event loop can retry on a later turn. A synchronous caller can wait with `iox.Backoff`. [Queue operations](../../README.md#operations)

## Dependencies

- `atomix` - Atomic primitives with explicit memory ordering
- `iox` - Semantic errors
- `spin` - Adaptive wait
