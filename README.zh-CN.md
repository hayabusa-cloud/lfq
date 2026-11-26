# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Benchmarks](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**语言:** [English](README.md) | 简体中文 | [Español](README.es.md) | [日本語](README.ja.md) | [Français](README.fr.md)

Go 语言的无锁和无等待 FIFO 队列实现。

## 概述

`lfq` 包提供了针对不同生产者/消费者模式优化的有界 FIFO 队列。每种变体都使用适合其访问模式的最有效的并发算法，确保在极高竞争下依然保持健壮和可预测的扩展性。

### 理论与背景

`lfq` 建立在经典的并发计算研究之上，提供严格有界的、无锁（Lock-free）和无等待（Wait-free）队列：

- SPSC 队列使用 **Lamport 环形缓冲区（1977）**。生产者和消费者各自写入自己的游标，每次入队或出队最多读取一次对方的游标。
- **Scalable Circular Queue（SCQ，2019）**。泛型 FAA 队列基于 Ruslan Nikolaev 的 SCQ 算法。

### `iox` 非阻塞 I/O 栈

队列操作返回 `ErrWouldBlock` 时，事件循环可以先处理其他工作，稍后重试。同步调用方可以使用 `iox.Backoff` 等待。

```go
// 直接构造函数（推荐用于大多数场景）
q := lfq.NewSPSC[Event](1024)

// Builder API - 根据约束自动选择算法
q := lfq.Build[Event](lfq.New(1024).SingleProducer().SingleConsumer())  // → SPSC
q := lfq.Build[Event](lfq.New(1024).SingleConsumer())                   // → MPSC
q := lfq.Build[Event](lfq.New(1024).SingleProducer().Compact())         // → SPMCSeq
q := lfq.Build[Event](lfq.New(1024).Compact())                          // → MPMCSeq
```

## 安装

```bash
go get code.hybscloud.com/lfq
```

**要求:** Go 1.27+

### 编译器要求

生产构建和基准测试使用 Atomix Go 编译器。[Atomix Go](https://github.com/hayabusa-cloud/go)

```bash
# 使用 Makefile（推荐）
make install-compiler   # Linux 的 amd64/arm64 或 macOS 的 arm64
make build              # 使用内部函数编译器构建
make test               # 使用内部函数编译器测试

# 或从源代码构建编译器
make install-compiler-source
```

两个安装目标默认使用 Go 1.27.x 系列最新发布的 Atomix 编译器。要选择特定版本，请设置环境变量 `COMPILER_TAG`。查询版本使用 `jq`，可通过 `GH_TOKEN` 向 GitHub 验证身份。从源码构建时，请在 `PATH` 中使用官方 Go 1.27.x 的最新版本。

手动安装时，请下载对应平台的归档文件，用同一版本的 `SHA256SUMS` 校验，然后将解压出的 `go` 目录放在 `$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}`。[编译器发布页](https://github.com/hayabusa-cloud/go/releases)

```bash
# 构建使用 lfq 的应用程序
compiler="$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}"
GOTOOLCHAIN=local GOROOT="$compiler" "$compiler/bin/go" build ./...
```

Atomix 编译器会内联 `atomix` 操作。CI 使用标准 Go 编译器运行正确性测试。

## 基准测试

CI 在原生 Linux amd64 和 arm64 上使用 Atomix 编译器和 `GOMAXPROCS=2` 运行 `make bench-ci`。Actions 摘要显示每项基准十次采样的中位数，涵盖会话交接与重试、四种队列各执行一次入队和出队，以及 `SPMCIndirectSeq` 多消费者并发出队。原始样本保存在工作流产物中。

`Handoff` 在两个 goroutine 之间传递一个值，分别使用 `sess` 的四槽 SPSC 数据队列和相同容量的 Go 缓冲通道。两者在队列满或空时都使用 `iox.Backoff`。`ns/op` 包含等待时间，`waits/op` 表示每传递一个值时两端等待次数之和。

`FullRetry` 在单个 goroutine 中测量向已满队列入队失败、出队一个值并成功重试入队的过程。运行 `make bench-session` 可执行会话基准测试，运行 `make bench` 可执行全部基准测试。

在 amd64 和 arm64 上，以下基准测试在同一个 goroutine 中比较 `SPSC[uintptr]`（Go）和 `SPSCIndirect`（汇编）。`EnqueueDequeue` 测量一次入队紧接一次出队的总耗时。`EnqueueWithBatchDrain` 给出每次入队的平均耗时，其中包括每入队 1024 次后排空队列的时间。类型化队列在出队时将槽位置零。间接队列会保留槽中的值，直到复用该槽位。

```bash
make bench-ci BENCH_PATTERN='^BenchmarkSPSC(TypedGo|IndirectAsm)'
```

## 队列类型

| 类型 | 模式 | 进度保证 | 使用场景 |
|------|------|---------|---------|
| **SPSC** | 单生产者单消费者 | 无等待 | 流水线阶段、通道 |
| **MPSC** | 多生产者单消费者 | 无锁（出队无等待） | 事件聚合、日志 |
| **SPMC** | 单生产者多消费者 | 无锁（入队无等待） | 任务分发 |
| **MPMC** | 多生产者多消费者 | 无锁 | 通用场景 |

### 进度保证

- 即使其他调用方都暂停，**无等待**（wait-free）操作仍能由调用方在有限步内完成。
- 在**无锁**（lock-free）队列中，只要有调用方持续执行队列操作，就一定有调用能够完成。单个调用可能一直无法完成。

返回 `ErrWouldBlock` 表示本次调用已结束，没有传递元素。

## 算法

### SPSC: Lamport 环形缓冲区

只有生产者推进队尾，只有消费者推进队头。双方都缓存对方的游标，每次调用最多刷新一次。入队和出队都是无等待操作，在 O(1) 步内完成。

```go
q := lfq.NewSPSC[int](1024)

// 生产者
value := 42
fmt.Println(q.Enqueue(&value)) // <nil>

// 消费者
fmt.Println(q.Dequeue()) // 42 <nil>

elem, err := q.Dequeue()
fmt.Println(elem, lfq.IsWouldBlock(err)) // 0 true
```

### MPSC/SPMC/MPMC: 基于 FAA（默认）

默认的多方队列使用 fetch-and-add（FAA）在包含 `2n` 个槽位的环中预留位置。`MPMCIndirect` 和 `MPMCPtr` 按连续的槽位位置依次发布和取出值。如果预留下一个位置的调用方暂停，其他调用方可以使用该位置。

- **权衡**：名义容量为 `n` 时需要 `2n` 个物理槽位。
- **瞬时容量**：当有 `P` 个并发生产者时，在背压生效前，可能会短暂入队最多 `P-1` 个超过 `Cap()` 的额外元素。

使用 `MPSC[T]`、`MPSCIndirect` 或 `MPSCPtr` 时，同时进行的 `Enqueue` 调用最多为 `Cap()` 个，已暂停的生产者仍计入其中。

共享 `MPMC[T]`、`SPMC[T]`、`SPMCIndirect` 或 `SPMCPtr` 时，每批使用一个新队列，最多放入 `Cap()` 个元素，然后按[单消费者排空流程](#优雅关闭)取出。如果消费者需要在生产期间轮询，或多个消费者需要并发出队，请使用对应的 `Seq` 队列。

```go
// 多生产者，单消费者
q := lfq.NewMPSC[Event](1024)  // FAA 生产者，无等待出队

// 一个生产者将一批元素放入新队列
q := lfq.NewSPMC[Task](1024)

// 多个生产者将一批元素放入新队列
q := lfq.NewMPMC[*Request](4096)
```

基于周期的槽位验证提供 ABA 安全性，无需纪元计数器或风险指针。

### Indirect/Ptr 变体: 128 位原子操作

Indirect 和 Ptr 队列变体（除 Compact Indirect 外的所有非 SPSC 变体）将序列号和载荷打包到单个 128 位原子操作中。这减少了缓存行争用，提高了高并发下的吞吐量。

对于将指针编码到 128 位槽位中的 Ptr 变体（FAA 和 PtrSeq），调用方须保留每个 Go 对象的有类型引用并保持其地址稳定，直到消费者取得所有权。

```go
// Indirect 将序列号和值存储在同一个原子对中。
q := lfq.NewMPMCIndirect(4096)

// Ptr - unsafe.Pointer 同样优化
q := lfq.NewMPMCPtr(4096)
```

## Builder API

基于约束的自动算法选择：

```go
// SPSC - 两个约束 → Lamport 环形缓冲区
q := lfq.Build[T](lfq.New(1024).SingleProducer().SingleConsumer())

// MPSC - 仅单消费者
q := lfq.Build[T](lfq.New(1024).SingleConsumer())

// SPMC - 仅单生产者
q := lfq.Build[T](lfq.New(1024).SingleProducer())

// MPMC - 默认构建器选项
q := lfq.Build[T](lfq.New(1024))
```

## 变体

每种队列类型有三种变体：

| 变体 | 元素类型 | 使用场景 |
|------|---------|---------|
| Generic | `[T any]` | 类型安全，通用 |
| Indirect | `uintptr` | 基于索引的池，句柄 |
| Ptr | `unsafe.Pointer` | 零拷贝指针传递 |

```go
// 泛型
q := lfq.NewMPMC[MyStruct](1024)

// 间接 - 用于池索引
q := lfq.NewMPMCIndirect(1024)
q.Enqueue(uintptr(poolIndex))

// 指向包级消息数组中元素的指针
q := lfq.NewMPMCPtr(len(messages))
q.Enqueue(unsafe.Pointer(&messages[i]))
```

### 紧凑模式

Compact() 选择基于 CAS 的算法，使用 n 个物理槽位（而非基于 FAA 默认的 2n 个）。当内存效率比争用可扩展性更重要时使用：

```go
// 紧凑模式 - 基于 CAS，n 个槽位
q := lfq.New(4096).Compact().BuildIndirect()
```

| 模式 | 算法 | 物理槽位数 | 使用场景 |
|------|------|-----------|---------|
| 默认 | 基于 FAA | 2n | 高争用，可扩展性 |
| 紧凑 | 基于 CAS | n | 内存受限 |

SPSC 变体已使用 n 个槽位的 Lamport 环形缓冲区，因此会忽略 `Compact()`。在其他拓扑中，`Compact().BuildIndirect()` 接受 63 位值。`SPMCCompactIndirect` 和 `MPMCCompactIndirect` 还要求，在多个消费者可能同时观察到这些值期间，各值必须互不相同。

多个消费者需要处理重复的 `uintptr` 值时，单生产者使用 `NewSPMCIndirectSeq`，多生产者使用 `NewMPMCIndirect`。

## 操作

| 操作 | 返回值 | 描述 |
|------|-------|------|
| `Enqueue(elem)` | `error` | 入队成功时返回 `nil`，否则返回 `ErrWouldBlock` |
| `Dequeue()` | `(T, error)` | 返回下一个元素和 `nil`，或返回零值和 `ErrWouldBlock` |
| `Cap()` | `int` | 队列容量 |

在默认的 MPSC 队列、`MPSCSeq[T]` 和 `MPMCSeq[T]` 中，生产者尚未发布队首元素时，即使后续的入队操作已经成功，`Dequeue` 也可能返回 `ErrWouldBlock`。在 `SPMCSeq[T]` 和 `MPMCSeq[T]` 中，消费者尚未释放下一个待写入的槽位时，即使后续的出队操作已经成功，`Enqueue` 也可能返回 `ErrWouldBlock`。

### 错误处理

SPSC 队列已满时，生产者保留未能入队的 `3`。消费者取出 `1` 后，生产者重试入队，消费者随后依次取出 `2` 和 `3`。

```go
q := lfq.NewSPSC[int](2)
q.Enqueue(new(1))
q.Enqueue(new(2))

pending := 3
fmt.Println(lfq.IsWouldBlock(q.Enqueue(&pending))) // true
fmt.Println(q.Dequeue())                           // 1 <nil>
fmt.Println(q.Enqueue(&pending))                   // <nil>

for {
	item, err := q.Dequeue()
	if lfq.IsWouldBlock(err) {
		break
	}
	fmt.Println(item)
}
```

## 使用模式

以下模式展示了如何将 `lfq` 集成到并发系统中。通过避免热路径上的内存分配并使用合适的队列变体，您可以大幅降低延迟。

### 缓冲池

由一个 goroutine 借用缓冲区，一个 goroutine 归还。每次 `alloc` 成功都会在空闲队列中腾出一个位置。最后一次使用缓冲区后，调用一次 `free` 归还对应的索引。

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

### 事件聚合

聚合器在服务运行期间持续工作。每个生产者保留当前读数，直到成功入队。

```go
type Event struct {
	Source    string
	Timestamp time.Time
	Data      any
}

// 多源 → 单处理器
events := lfq.NewMPSC[Event](max(8192, len(sensors)))

// 事件源（多生产者）
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

// 单聚合器（单消费者）
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

### 背压处理

调用方可定义 `func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxAttempts int) bool`。首次入队也计入 `maxAttempts`，上限为零时立即返回 `false`。

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

### 优雅关闭

要从新初始化的队列中取出一批数据，先等待每个生产者的最后一次 `Enqueue()` 调用返回。如果队列支持 `Drain()`，此时调用它。随后由一个消费者首次调用 `Dequeue()`，并继续出队，直到收到 `ErrWouldBlock`。

```go
// 生产者协程结束
prodWg.Wait()

// 如果用 Init 初始化了队列变量，请用该变量的地址作为 q。
if d, ok := any(q).(lfq.Drainer); ok {
	d.Drain()
}

// 持续出队，直到队列报告为空。
for {
	item, err := q.Dequeue()
	if err != nil {
		break
	}
	process(item)
}
```

`Drain()` 会禁用基于 FAA 的泛型 MPMC 队列和 SPMC 队列的出队阈值。在基于 FAA 的 MPSC 队列中，它表示生产已经结束。对 `MPMCIndirect` 和 `MPMCPtr` 调用 `Drain()` 没有任何效果。SPSC 队列可以直接出队。

## 如何选择队列

```
┌─────────────────────────────────────────────────────────────────┐
│                         有多少生产者？                          │
│                                                                 │
│      ┌──────────────────┐          ┌────────────────────┐       │
│      │   一个 (SPSC/    │          │    多个 (MPMC/     │       │
│      │   SPMC)          │          │    MPSC)           │       │
│      └────────┬─────────┘          └─────────┬──────────┘       │
│               │                               │                 │
│               ▼                               ▼                 │
│   ┌──────────────────┐              ┌──────────────────┐        │
│   │ 一个消费者？     │              │ 一个消费者？     │        │
│   └────────┬─────────┘              └────────┬─────────┘        │
│    是      │     否                  是      │     否           │
│     │      │      │                   │      │      │           │
│     ▼      │      ▼                   ▼      │      ▼           │
│  SPSC[T]   │  SPMCSeq[T]           MPSC[T]   │  MPMCSeq[T]      │
│            │                                 │                  │
└────────────┴─────────────────────────────────┴──────────────────┘

变体选择：
• Generic [T]     → 类型安全，值拷贝语义
• Indirect        → 池索引，缓冲区偏移量 (uintptr)
• Ptr             → 零拷贝对象传递 (unsafe.Pointer)
```

### 容量

容量向上取整到下一个 2 的幂次方：

```go
q := lfq.NewMPMC[int](3)     // 实际容量: 4
q := lfq.NewMPMC[int](4)     // 实际容量: 4
q := lfq.NewMPMC[int](1000)  // 实际容量: 1024
q := lfq.NewMPMC[int](1024)  // 实际容量: 1024
```

最小容量为 `2`。当 `capacity < 2` 时，构造函数会 panic。

## 内存布局

所有队列使用缓存行填充（64 字节）防止伪共享：

```go
type MPMC[T any] struct {
    _        [64]byte      // 填充
    tail     atomix.Uint64 // 生产者索引
    _        [64]byte      // 填充
    head     atomix.Uint64 // 消费者索引
    _        [64]byte      // 填充
    buffer   []slot[T]
    // ...
}
```

## 竞态检测

Go 的竞态检测器无法观察 `atomix` 汇编实现中的所有同步操作。运行 `go test -race ./...` 可执行带竞态检测的测试。运行 `go test ./...` 还会执行竞态检测模式下跳过的并发测试和示例。

## 依赖

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — 语义错误 (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — 显式内存顺序的原子原语
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — 自旋原语

## 平台支持

| 平台 | 状态 |
|------|------|
| linux/amd64, linux/arm64 (LSE2) | Linux 6.18 及以上版本的生产环境 |
| linux/riscv64, linux/loong64 | CI 交叉编译 |
| darwin/amd64, darwin/arm64 | 开发环境。CI 在 darwin/arm64 上运行测试。 |
| freebsd/amd64, freebsd/arm64 | 开发环境 |

## 参考文献

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *DISC 2019 (LIPIcs)*. https://doi.org/10.4230/LIPIcs.DISC.2019.28. Preprint: https://arxiv.org/abs/1908.04511.
- Lamport, L. (1977). Proving the Correctness of Multiprocess Programs. *IEEE Transactions on Software Engineering*, 3(2), 125–143.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## 许可证

MIT — 参见 [LICENSE](./LICENSE)。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
