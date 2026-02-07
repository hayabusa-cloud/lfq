# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/lfq)](https://goreportcard.com/report/github.com/hayabusa-cloud/lfq)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**言語:** [English](README.md) | [简体中文](README.zh-CN.md) | 日本語 | [Español](README.es.md) | [Français](README.fr.md)

Go 向けのロックフリーおよびウェイトフリー FIFO キュー実装。

## 概要

lfq は、異なるプロデューサー/コンシューマーパターン向けに最適化された有界 FIFO キューを提供します。各バリアントはそのアクセスパターンに適したアルゴリズムを使用します。

```go
// 直接コンストラクタ（ほとんどの場合に推奨）
q := lfq.NewSPSC[Event](1024)

// Builder API - 制約に基づいてアルゴリズムを自動選択
q := lfq.Build[Event](lfq.New(1024).SingleProducer().SingleConsumer())  // → SPSC
q := lfq.Build[Event](lfq.New(1024).SingleConsumer())                   // → MPSC
q := lfq.Build[Event](lfq.New(1024).SingleProducer())                   // → SPMC
q := lfq.Build[Event](lfq.New(1024))                                    // → MPMC
```

## インストール

```bash
go get code.hybscloud.com/lfq
```

**要件:** Go 1.25+

### コンパイラ要件

より高いパフォーマンスを得るには、[内部関数最適化 Go コンパイラ](https://github.com/hayabusa-cloud/go)でコンパイルしてください：

```bash
# Makefile を使用（推奨）
make install-compiler   # ビルド済みリリースをダウンロード（約 30 秒）
make build              # 内部関数コンパイラでビルド
make test               # 内部関数コンパイラでテスト

# またはソースからコンパイラをビルド（最新開発版）
make install-compiler-source
```

手動インストール：

```bash
# ビルド済みリリース（推奨）
URL=$(curl -fsSL https://api.github.com/repos/hayabusa-cloud/go/releases/latest | grep 'browser_download_url.*linux-amd64\.tar\.gz"' | cut -d'"' -f4)
curl -fsSL "$URL" | tar -xz -C ~/sdk
mv ~/sdk/go ~/sdk/go-atomix

# lfq 依存コードのビルドに使用
GOROOT=~/sdk/go-atomix ~/sdk/go-atomix/bin/go build ./...
```

内部関数コンパイラは atomix 操作を適切なメモリオーダリングでインライン化します。標準 Go コンパイラは基本的なテストには使用できますが、高競合下では問題が発生する可能性があります。

## キュータイプ

| タイプ | パターン | 進行保証 | ユースケース |
|--------|----------|----------|--------------|
| **SPSC** | 単一プロデューサー単一コンシューマー | ウェイトフリー | パイプラインステージ、チャネル |
| **MPSC** | 複数プロデューサー単一コンシューマー | ロックフリー | イベント集約、ロギング |
| **SPMC** | 単一プロデューサー複数コンシューマー | ロックフリー | タスク分散 |
| **MPMC** | 複数プロデューサー複数コンシューマー | ロックフリー | 汎用 |

### 進行保証

- **ウェイトフリー**: すべての操作が有界ステップで完了 (O(1))
- **ロックフリー**: システム全体の進行を保証；少なくとも1つのスレッドが進行

## アルゴリズム

### SPSC: Lamport リングバッファ

キャッシュインデックス最適化を備えた古典的な有界バッファ。

```go
q := lfq.NewSPSC[int](1024)

// プロデューサー
q.Enqueue(&value)  // ウェイトフリー O(1)

// コンシューマー
elem, err := q.Dequeue()  // ウェイトフリー O(1)
```

### MPSC/SPMC/MPMC: FAA ベース（デフォルト）

デフォルトでは、マルチアクセスキューは SCQ（スケーラブル循環キュー）由来の FAA（Fetch-And-Add）ベースのアルゴリズムを使用します。FAA は位置カウンターを盲目的にインクリメントするため、容量 n に対して 2n の物理スロットが必要ですが、高競合下では CAS ベースの代替案よりも優れたスケーラビリティを発揮します。

```go
// 複数プロデューサー、単一コンシューマー
q := lfq.NewMPSC[Event](1024)  // FAA プロデューサー、ウェイトフリーデキュー

// 単一プロデューサー、複数コンシューマー
q := lfq.NewSPMC[Task](1024)   // ウェイトフリーエンキュー、FAA コンシューマー

// 複数プロデューサーと複数コンシューマー
q := lfq.NewMPMC[*Request](4096)  // FAA ベース SCQ アルゴリズム
```

サイクルベースのスロット検証が ABA 安全性を提供。エポックカウンターやハザードポインターは不要。

### Indirect/Ptr バリアント: 128 ビットアトミック操作

Indirect および Ptr キューバリアント（非 SPSC、非 Compact）は、シーケンス番号と値を単一の 128 ビットアトミック操作にパックします。これによりキャッシュライン競合が軽減され、高並行性下でのスループットが向上します。

```go
// Indirect - 操作ごとに単一の 128 ビットアトミック
q := lfq.NewMPMCIndirect(4096)

// Ptr - unsafe.Pointer も同様に最適化
q := lfq.NewMPMCPtr(4096)
```

## Builder API

制約に基づく自動アルゴリズム選択：

```go
// SPSC - 両方の制約 → Lamport リング
q := lfq.Build[T](lfq.New(1024).SingleProducer().SingleConsumer())

// MPSC - 単一コンシューマーのみ
q := lfq.Build[T](lfq.New(1024).SingleConsumer())

// SPMC - 単一プロデューサーのみ
q := lfq.Build[T](lfq.New(1024).SingleProducer())

// MPMC - 制約なし（デフォルト）
q := lfq.Build[T](lfq.New(1024))
```

## バリアント

各キュータイプには3つのバリアントがあります：

| バリアント | 要素タイプ | ユースケース |
|------------|-----------|--------------|
| Generic | `[T any]` | 型安全、汎用 |
| Indirect | `uintptr` | インデックスベースのプール、ハンドル |
| Ptr | `unsafe.Pointer` | ゼロコピーポインター受け渡し |

```go
// ジェネリック
q := lfq.NewMPMC[MyStruct](1024)

// 間接 - プールインデックス用
q := lfq.NewMPMCIndirect(1024)
q.Enqueue(uintptr(poolIndex))

// ポインター - ゼロコピー
q := lfq.NewMPMCPtr(1024)
q.Enqueue(unsafe.Pointer(obj))
```

### コンパクトモード

Compact() は CAS ベースのアルゴリズムを選択し、n 物理スロット（FAA ベースのデフォルトの 2n ではなく）を使用します。競合スケーラビリティよりもメモリ効率を重視する場合に使用：

```go
// コンパクトモード - CAS ベース、n スロット
q := lfq.New(4096).Compact().BuildIndirect()
```

| モード | アルゴリズム | 物理スロット数 | 使用場面 |
|--------|-------------|---------------|---------|
| デフォルト | FAA ベース | 2n | 高競合、スケーラビリティ |
| コンパクト | CAS ベース | n | メモリ制約 |

SPSC バリアントは既に n スロット（Lamport リングバッファ）を使用しており、Compact() を無視します。Compact() を使用した Indirect キューでは、値は 63 ビットに制限されます。

## 操作

| 操作 | 戻り値 | 説明 |
|------|--------|------|
| `Enqueue(elem)` | `error` | 要素を追加；満杯時は `ErrWouldBlock` を返す |
| `Dequeue()` | `(T, error)` | 要素を削除；空の場合は `ErrWouldBlock` を返す |
| `Cap()` | `int` | キュー容量 |

### エラー処理

```go
err := q.Enqueue(&item)
if lfq.IsWouldBlock(err) {
    // キューが満杯 - バックプレッシャーまたはリトライ
}

elem, err := q.Dequeue()
if lfq.IsWouldBlock(err) {
    // キューが空 - 待機またはポーリング
}
```

## 使用パターン

### バッファプール

```go
const poolSize = 1024
const bufSize = 4096

// バッファを事前確保
pool := make([][]byte, poolSize)
for i := range pool {
    pool[i] = make([]byte, bufSize)
}

// フリーリストで利用可能なインデックスを追跡
freeList := lfq.NewSPSCIndirect(poolSize)
for i := range poolSize {
    freeList.Enqueue(uintptr(i))
}

// 確保
func Alloc() ([]byte, uintptr, bool) {
    idx, err := freeList.Dequeue()
    if err != nil {
        return nil, 0, false
    }
    return pool[idx], idx, true
}

// 解放
func Free(idx uintptr) {
    freeList.Enqueue(idx)
}
```

### イベント集約

```go
type Event struct {
    Source    string
    Timestamp time.Time
    Data      any
}

// 複数ソース → 単一プロセッサ
events := lfq.NewMPSC[Event](8192)

// イベントソース（複数プロデューサー）
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

// 単一アグリゲーター（単一コンシューマー）
go func() {
    for {
        ev, err := events.Dequeue()
        if err == nil {
            aggregate(*ev)
        }
    }
}()
```

### バックプレッシャー処理

```go
// リトライとyieldによるエンキュー
func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxRetries int) bool {
	ba := iox.Backoff{}
    for i := range maxRetries {
        if q.Enqueue(&item) == nil {
            return true
        }
        ba.Wait() // コンシューマーにドレインを許可
    }
    return false // 呼び出し元にバックプレッシャーを適用
}
```

### グレースフルシャットダウン

FAA ベースのキュー（MPMC、SPMC、MPSC）にはライブロック防止のための閾値メカニズムがあります。プロデューサーがコンシューマーより先に終了するグレースフルシャットダウンでは、`Drainer` インターフェースを使用します：

```go
// プロデューサーゴルーチンが終了
prodWg.Wait()

// これ以上エンキューしないことを通知
if d, ok := q.(lfq.Drainer); ok {
    d.Drain()
}

// コンシューマーは閾値ブロッキングなしで
// 残りの全アイテムをドレインできる
for {
    item, err := q.Dequeue()
    if err != nil {
        break // キューが空
    }
    process(item)
}
```

`Drain()` はヒントです — 呼び出し元は以降 `Enqueue()` を呼び出さないことを保証する必要があります。SPSC キューは閾値メカニズムがないため `Drainer` を実装しません。型アサーションがこのケースを自然に処理します。

## キュー選択ガイド

```
┌─────────────────────────────────────────────────────────────────┐
│                    プロデューサーの数は？                         │
│                                                                 │
│      ┌──────────────────┐          ┌────────────────────┐      │
│      │    1つ (SPSC/     │          │   複数 (MPMC/      │      │
│      │    SPMC)          │          │   MPSC)            │      │
│      └────────┬─────────┘          └─────────┬──────────┘      │
│               │                               │                 │
│               ▼                               ▼                 │
│   ┌──────────────────┐              ┌──────────────────┐       │
│   │ コンシューマーは  │              │ コンシューマーは  │       │
│   │ 1つ？            │              │ 1つ？            │       │
│   └────────┬─────────┘              └────────┬─────────┘       │
│    はい    │     いいえ              はい    │     いいえ      │
│     │      │      │                   │      │      │          │
│     ▼      │      ▼                   ▼      │      ▼          │
│   SPSC     │    SPMC                MPSC     │    MPMC         │
│            │                                 │                  │
└────────────┴─────────────────────────────────┴─────────────────┘

バリアント選択：
• Generic [T]     → 型安全、コピーセマンティクス
• Indirect        → プールインデックス、バッファオフセット (uintptr)
• Ptr             → ゼロコピーオブジェクト受け渡し (unsafe.Pointer)
```

### 容量

容量は次の 2 の累乗に切り上げられます：

```go
q := lfq.NewMPMC[int](3)     // 実際の容量: 4
q := lfq.NewMPMC[int](4)     // 実際の容量: 4
q := lfq.NewMPMC[int](1000)  // 実際の容量: 1024
q := lfq.NewMPMC[int](1024)  // 実際の容量: 1024
```

## メモリレイアウト

すべてのキューはフォルスシェアリングを防ぐためにキャッシュラインパディング（64 バイト）を使用：

```go
type MPMC[T any] struct {
    _        [64]byte      // パディング
    tail     atomix.Uint64 // プロデューサーインデックス
    _        [64]byte      // パディング
    head     atomix.Uint64 // コンシューマーインデックス
    _        [64]byte      // パディング
    buffer   []slot[T]
    // ...
}
```

## 競合検出

Go の競合検出器はロックフリーアルゴリズムの検証には設計されていません。明示的な同期プリミティブ（mutex、channel）を追跡しますが、アトミックメモリオーダリングによる happens-before 関係は観察できません。

テストは2つの保護機構を使用：
- ビルドタグ `//go:build !race` でサンプルファイルを競合テストから除外
- ランタイムチェック `if lfq.RaceEnabled { t.Skip() }` で `lockfree_test.go` の並行テストをスキップ

`go test -race ./...` で競合安全なテストを実行、`go test ./...` ですべてのテストを実行します。

## 依存関係

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — セマンティックエラー (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — 明示的メモリオーダリングのアトミックプリミティブ
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — スピンプリミティブ

## プラットフォームサポート

| プラットフォーム | ステータス |
|------------------|-----------|
| linux/amd64 | プライマリ |
| linux/arm64 | サポート |
| linux/riscv64 | サポート |
| linux/loong64 | サポート |
| darwin/amd64, darwin/arm64 | サポート |
| freebsd/amd64, freebsd/arm64 | サポート |

## 参考文献

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *arXiv*, arXiv:1908.04511. https://arxiv.org/abs/1908.04511.
- Lamport, L. (1974). A New Solution of Dijkstra's Concurrent Programming Problem. *Communications of the ACM*, 17(8), 453–455.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## ライセンス

MIT — [LICENSE](./LICENSE) を参照。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
