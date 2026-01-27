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

## Review Checklist

### Algorithm

- [ ] FAA default, CAS only with `Compact()`
- [ ] FAA = 2n slots, CAS = n slots, SPSC = n
- [ ] Threshold mechanism for FAA MPMC/SPMC only

### Memory Ordering

- [ ] `LoadAcquire`/`StoreRelease` pairs correct
- [ ] `AddAcquire` for FAA position claiming
- [ ] 64-byte padding between head/tail

### Atomic Operation Retry Loops

- [ ] Uses `spin.Wait{}` with `sw.Once()`
- [ ] No raw loops without backoff

### Builders

- [ ] `BuildSPSC` requires SP + SC
- [ ] `BuildMPSC` requires SC only
- [ ] `BuildSPMC` requires SP only

## Common Issues

- Wrong slot count (must be 2n for FAA)
- Missing `spin.Wait` in retry loops
- Incorrect cycle calculation (`position / size` not `/ capacity`)
- Compact Indirect values must be < 63 bits

## Error Handling

`ErrWouldBlock` = queue full/empty. Control flow, not failure.

## Dependencies

- `atomix` - Atomic primitives (requires intrinsics compiler)
- `iox` - Semantic errors
- `spin` - Adaptive wait
