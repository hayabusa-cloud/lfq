# lfq - Copilot Instructions

Lock-free FIFO queues.

## Compiler Requirement

ALL builds and tests MUST use the intrinsics-optimized Go compiler:

```bash
# Set up atomix compiler (github.com/hayabusa-cloud/go, branch: atomix)
export ATOMIX_GOROOT=<path-to-atomix-go>
export GOROOT=$ATOMIX_GOROOT
export PATH=$ATOMIX_GOROOT/bin:$PATH
```

Standard Go compiler will compile but with degraded performance (~2.5ns overhead per atomic operation).

## Quick Reference

| Queue | Progress | Default (FAA) | Compact (CAS) |
|-------|----------|---------------|---------------|
| SPSC | Wait-free | n slots | Same |
| MPSC/SPMC/MPMC | Lock-free | 2n slots | n slots |

## Review Focus

Lock-free algorithm correctness, not code style. Key areas:

- Memory ordering (acquire/release pairs)
- Slot count matches algorithm (see Quick Reference)
- Retry loops use `spin.Wait{}`

## Pitfalls

- Mixing FAA slot math (2n) with CAS slot math (n)
- Missing spin wait in retry loops

## Error Handling

`ErrWouldBlock` = queue full/empty. Control flow, not failure.

## Dependencies

- `atomix` - Atomic primitives (requires intrinsics compiler)
- `iox` - Semantic errors
- `spin` - Adaptive wait
