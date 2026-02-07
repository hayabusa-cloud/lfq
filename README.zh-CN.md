# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/lfq)](https://goreportcard.com/report/github.com/hayabusa-cloud/lfq)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**语言:** [English](README.md) | 简体中文 | [日本語](README.ja.md) | [Español](README.es.md) | [Français](README.fr.md)

Go 语言的无锁和无等待 FIFO 队列实现。

## 概述

lfq 提供针对不同生产者/消费者模式优化的有界 FIFO 队列。每种变体使用适合其访问模式的算法。

```go
// 直接构造函数（推荐用于大多数场景）
q := lfq.NewSPSC[Event](1024)

// Builder API - 根据约束自动选择算法
q := lfq.Build[Event](lfq.New(1024).SingleProducer().SingleConsumer())  // → SPSC
q := lfq.Build[Event](lfq.New(1024).SingleConsumer())                   // → MPSC
q := lfq.Build[Event](lfq.New(1024).SingleProducer())                   // → SPMC
q := lfq.Build[Event](lfq.New(1024))                                    // → MPMC
```

## 安装

```bash
go get code.hybscloud.com/lfq
```

**要求:** Go 1.25+

### 编译器要求

为获得更好的性能，请使用[内部函数优化 Go 编译器](https://github.com/hayabusa-cloud/go)编译：

```bash
# 使用 Makefile（推荐）
make install-compiler   # 下载预构建版本（约 30 秒）
make build              # 使用内部函数编译器构建
make test               # 使用内部函数编译器测试

# 或从源代码构建编译器（最新开发版）
make install-compiler-source
```

手动安装：

```bash
# 预构建版本（推荐）
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
URL=$(curl -fsSL https://api.github.com/repos/hayabusa-cloud/go/releases/latest | grep "browser_download_url.*${OS}-${ARCH}\.tar\.gz\"" | cut -d'"' -f4)
curl -fsSL "$URL" | tar -xz -C ~/sdk
mv ~/sdk/go ~/sdk/go-atomix

# 用于编译依赖 lfq 的代码
GOROOT=~/sdk/go-atomix ~/sdk/go-atomix/bin/go build ./...
```

内部函数编译器将 atomix 操作内联为正确内存顺序的指令。标准 Go 编译器可用于基本测试，但在高争用情况下可能出现问题。

## 队列类型

| 类型 | 模式 | 进度保证 | 使用场景 |
|------|------|---------|---------|
| **SPSC** | 单生产者单消费者 | 无等待 | 流水线阶段、通道 |
| **MPSC** | 多生产者单消费者 | 无锁 | 事件聚合、日志 |
| **SPMC** | 单生产者多消费者 | 无锁 | 任务分发 |
| **MPMC** | 多生产者多消费者 | 无锁 | 通用场景 |

### 进度保证

- **无等待 (Wait-free)**: 每个操作在有界步骤内完成 (O(1))
- **无锁 (Lock-free)**: 保证系统级进度；至少一个线程能够前进

## 算法

### SPSC: Lamport 环形缓冲区

经典有界缓冲区，带缓存索引优化。

```go
q := lfq.NewSPSC[int](1024)

// 生产者
q.Enqueue(&value)  // 无等待 O(1)

// 消费者
elem, err := q.Dequeue()  // 无等待 O(1)
```

### MPSC/SPMC/MPMC: 基于 FAA（默认）

默认情况下，多访问队列使用基于 FAA（Fetch-And-Add）的算法，源自 SCQ（可扩展环形队列）。FAA 盲目递增位置计数器，对于容量 n 需要 2n 个物理槽位，但在高争用情况下比基于 CAS 的替代方案扩展性更好。

```go
// 多生产者，单消费者
q := lfq.NewMPSC[Event](1024)  // FAA 生产者，无等待出队

// 单生产者，多消费者
q := lfq.NewSPMC[Task](1024)   // 无等待入队，FAA 消费者

// 多生产者和消费者
q := lfq.NewMPMC[*Request](4096)  // 基于 FAA 的 SCQ 算法
```

基于周期的槽位验证提供 ABA 安全性，无需纪元计数器或风险指针。

### Indirect/Ptr 变体: 128 位原子操作

Indirect 和 Ptr 队列变体（非 SPSC、非 Compact）将序列号和值打包到单个 128 位原子操作中。这减少了缓存行争用，提高了高并发下的吞吐量。

```go
// Indirect - 每次操作单个 128 位原子
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

// MPMC - 无约束（默认）
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

// 指针 - 零拷贝
q := lfq.NewMPMCPtr(1024)
q.Enqueue(unsafe.Pointer(obj))
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

SPSC 变体已使用 n 个槽位（Lamport 环形缓冲区），会忽略 Compact()。对于使用 Compact() 的 Indirect 队列，值限制为 63 位。

## 操作

| 操作 | 返回值 | 描述 |
|------|-------|------|
| `Enqueue(elem)` | `error` | 添加元素；队列满时返回 `ErrWouldBlock` |
| `Dequeue()` | `(T, error)` | 移除元素；队列空时返回 `ErrWouldBlock` |
| `Cap()` | `int` | 队列容量 |

### 错误处理

```go
err := q.Enqueue(&item)
if lfq.IsWouldBlock(err) {
    // 队列已满 - 背压或重试
}

elem, err := q.Dequeue()
if lfq.IsWouldBlock(err) {
    // 队列为空 - 等待或轮询
}
```

## 使用模式

### 缓冲池

```go
const poolSize = 1024
const bufSize = 4096

// 预分配缓冲区
pool := make([][]byte, poolSize)
for i := range pool {
    pool[i] = make([]byte, bufSize)
}

// 空闲列表追踪可用索引
freeList := lfq.NewSPSCIndirect(poolSize)
for i := range poolSize {
    freeList.Enqueue(uintptr(i))
}

// 分配
func Alloc() ([]byte, uintptr, bool) {
    idx, err := freeList.Dequeue()
    if err != nil {
        return nil, 0, false
    }
    return pool[idx], idx, true
}

// 释放
func Free(idx uintptr) {
    freeList.Enqueue(idx)
}
```

### 事件聚合

```go
type Event struct {
    Source    string
    Timestamp time.Time
    Data      any
}

// 多源 → 单处理器
events := lfq.NewMPSC[Event](8192)

// 事件源（多生产者）
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

// 单聚合器（单消费者）
go func() {
    for {
        ev, err := events.Dequeue()
        if err == nil {
            aggregate(*ev)
        }
    }
}()
```

### 背压处理

```go
// 带重试和让步的入队
func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxRetries int) bool {
	ba := iox.Backoff{}
    for i := range maxRetries {
        if q.Enqueue(&item) == nil {
            return true
        }
        ba.Wait() // 让步以允许消费者消耗
    }
    return false // 对调用者施加背压
}

```

### 优雅关闭

FAA 基队列（MPMC、SPMC、MPSC）包含防止活锁的阈值机制。在生产者先于消费者结束的优雅关闭场景中，使用 `Drainer` 接口：

```go
// 生产者协程结束
prodWg.Wait()

// 通知不再有入队操作
if d, ok := q.(lfq.Drainer); ok {
    d.Drain()
}

// 消费者现在可以在没有阈值阻塞的情况下
// 消耗所有剩余项目
for {
    item, err := q.Dequeue()
    if err != nil {
        break // 队列为空
    }
    process(item)
}
```

`Drain()` 是一个提示 — 调用者必须确保之后不再调用 `Enqueue()`。SPSC 队列没有阈值机制，因此不实现 `Drainer`；类型断言会自然处理这种情况。

## 如何选择队列

```
┌─────────────────────────────────────────────────────────────────┐
│                      有多少生产者？                               │
│                                                                 │
│      ┌──────────────────┐          ┌────────────────────┐      │
│      │    一个 (SPSC/    │          │   多个 (MPMC/      │      │
│      │    SPMC)          │          │   MPSC)            │      │
│      └────────┬─────────┘          └─────────┬──────────┘      │
│               │                               │                 │
│               ▼                               ▼                 │
│   ┌──────────────────┐              ┌──────────────────┐       │
│   │ 一个消费者？      │              │ 一个消费者？      │       │
│   └────────┬─────────┘              └────────┬─────────┘       │
│    是      │     否                  是      │     否          │
│     │      │      │                   │      │      │          │
│     ▼      │      ▼                   ▼      │      ▼          │
│   SPSC     │    SPMC                MPSC     │    MPMC         │
│            │                                 │                  │
└────────────┴─────────────────────────────────┴─────────────────┘

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

Go 的竞态检测器不适用于无锁算法验证。它跟踪显式同步原语（mutex、channel），但无法观察原子内存顺序建立的 happens-before 关系。

测试使用两种保护机制：
- 构建标签 `//go:build !race` 将示例文件排除在竞态测试之外
- 运行时检查 `if lfq.RaceEnabled { t.Skip() }` 跳过 `lockfree_test.go` 中的并发测试

运行 `go test -race ./...` 执行竞态安全测试，或运行 `go test ./...` 执行所有测试。

## 依赖

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — 语义错误 (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — 显式内存顺序的原子原语
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — 自旋原语

## 平台支持

| 平台 | 状态 |
|------|------|
| linux/amd64 | 主要 |
| linux/arm64 | 支持 |
| linux/riscv64 | 支持 |
| linux/loong64 | 支持 |
| darwin/amd64, darwin/arm64 | 支持 |
| freebsd/amd64, freebsd/arm64 | 支持 |

## 参考文献

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *arXiv*, arXiv:1908.04511. https://arxiv.org/abs/1908.04511.
- Lamport, L. (1974). A New Solution of Dijkstra's Concurrent Programming Problem. *Communications of the ACM*, 17(8), 453–455.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## 许可证

MIT — 参见 [LICENSE](./LICENSE)。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
