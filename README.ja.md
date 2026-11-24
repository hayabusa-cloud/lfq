# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Benchmarks](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**言語:** [English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | 日本語 | [Français](README.fr.md)

Go 向けのロックフリーおよびウェイトフリー FIFO キュー実装。

## 概要

`lfq` パッケージは、ロックフリーおよびウェイトフリーな有界 FIFO キューを提供します。特定のプロデューサー/コンシューマーパターン（SPSC、MPSC、SPMC、MPMC）に最適化されており、高競合下でも予測可能でゼロアロケーションなスケーリングを保証します。

### 理論と背景

`lfq` は、従来のミューテックスや CAS（Compare-And-Swap）ループの限界を克服するために、基礎的な並行処理研究に基づいて構築されています：

- **Lamport リングバッファ (1977)**：単一プロデューサー/単一コンシューマー（SPSC）パスを駆動し、コストの高いアトミックな Read-Modify-Write 命令なしで厳密な $O(1)$ ウェイトフリーレイテンシを提供します。
- **Scalable Circular Queue（SCQ、2019）**。ジェネリックな FAA キューは Ruslan Nikolaev の SCQ アルゴリズムに基づいています。

### `iox` ノンブロッキング I/O スタック

キュー操作が `ErrWouldBlock` を返したら、イベントループはほかの処理を進めてから再試行できます。同期呼び出しでは `iox.Backoff` で待機できます。

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

**要件:** Go 1.27+

### コンパイラ要件

本番向けのビルドとベンチマークには Atomix Go コンパイラを使用します。[Atomix Go](https://github.com/hayabusa-cloud/go)

```bash
# Makefile を使用（推奨）
make install-compiler   # Linux の amd64/arm64 または macOS の arm64
make build              # 内部関数コンパイラでビルド
make test               # 内部関数コンパイラでテスト

# またはソースからコンパイラをビルド
make install-compiler-source
```

どちらのインストールターゲットも、デフォルトでは Go 1.27.x 向けの最新の Atomix コンパイラを使用します。特定のリリースを選ぶには、環境変数 `COMPILER_TAG` を設定してください。リリースの検索には `jq` を使用します。GitHub への認証には `GH_TOKEN` を指定できます。ソースからビルドする場合は、公式の Go 1.27.x の最新版を `PATH` に設定してください。

手動でインストールする場合は、使用するプラットフォーム用のアーカイブをダウンロードし、同じリリースの `SHA256SUMS` で検証してください。展開した `go` ディレクトリを `$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}` に配置します。[コンパイラのリリース一覧](https://github.com/hayabusa-cloud/go/releases)

```bash
# lfq を使うアプリケーションをビルド
compiler="$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}"
GOTOOLCHAIN=local GOROOT="$compiler" "$compiler/bin/go" build ./...
```

Atomix コンパイラは `atomix` の操作をインライン化します。CI では標準の Go コンパイラで動作を検証します。

## ベンチマーク

CI では Linux の amd64 と arm64 の実機で、Atomix コンパイラと `GOMAXPROCS=2` を使って `make bench-ci` を実行します。Actions のサマリーには、セッションの受け渡しと再試行、4 種類のキューそれぞれの enqueue/dequeue の組、複数のコンシューマが並行して取り出す SPMC について、各 10 回の測定の中央値を表示します。測定の生データはアーティファクトから取得できます。

`Handoff` は、`sess` が使う容量 4 の SPSC データキューと同じ容量の Go のバッファ付きチャネルで、それぞれ 2 つの goroutine 間で値を 1 個受け渡します。どちらも満杯または空のときは `iox.Backoff` で待機します。`ns/op` は待機時間を含み、`waits/op` は値 1 個を渡す間に送受信側が待機した回数の合計です。

`FullRetry` は、満杯のキューへの挿入が失敗し、値を 1 個取り出してから挿入に成功するまでを単一の goroutine で測定します。セッションに関する測定には `make bench-session`、すべてのベンチマークには `make bench` を使います。

## キュータイプ

| タイプ | パターン | 進行保証 | ユースケース |
|--------|----------|----------|--------------|
| **SPSC** | 単一プロデューサー単一コンシューマー | ウェイトフリー | パイプラインステージ、チャネル |
| **MPSC** | 複数プロデューサー単一コンシューマー | ロックフリー（デキューはウェイトフリー） | イベント集約、ロギング |
| **SPMC** | 単一プロデューサー複数コンシューマー | ロックフリー（エンキューはウェイトフリー） | タスク分散 |
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

デフォルトの複数アクセスキューは fetch-and-add（FAA）で `2n` 個のスロットを持つリングの位置を予約します。`MPMCIndirect` と `MPMCPtr` は連続するスロット位置に順に値を公開し、同じ順に取り出します。次の位置を予約した呼び出し元が停止しても、別の呼び出し元がその位置を使えます。

- **トレードオフ**: 名目容量 `n` に対して物理スロット `2n` が必要です。
- **一時容量**: `P` 個の並行プロデューサーがいる場合、バックプレッシャーが効く前に `Cap()` を最大 `P-1` 個一時的に超えてエンキューされることがあります。

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

Indirect および Ptr キューバリアント（非 SPSC のうち Compact Indirect を除く）は、シーケンス番号とペイロードを単一の 128 ビットアトミック操作にパックします。これによりキャッシュライン競合が軽減され、高並行性下でのスループットが向上します。

ポインターを 128 ビットスロットに格納する Ptr バリアント（FAA および PtrSeq）では、コンシューマーが所有権を引き継ぐまで、各 Go オブジェクトへの型付き参照を保持し、アドレスを固定してください。

```go
// Indirect はシーケンス番号と値を一つのアトミックなペアに格納します。
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

// パッケージ変数のメッセージ配列内の要素を指すポインター
q := lfq.NewMPMCPtr(len(messages))
q.Enqueue(unsafe.Pointer(&messages[i]))
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

SPSC はすでに n スロットの Lamport リングバッファを使うため、`Compact()` を無視します。それ以外の構成では、`Compact().BuildIndirect()` が扱える値は 63 ビットまでです。`SPMCCompactIndirect` と `MPMCCompactIndirect` では、複数のコンシューマーが値を観測し得る間、その値が互いに異なることも必要です。

複数のコンシューマーで重複する `uintptr` 値を扱う場合、プロデューサーが 1 つなら `NewSPMCIndirectSeq`、複数なら `NewMPMCIndirect` を使います。

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

以下のパターンは、`lfq` を並行システムに統合する方法を示しています。ホットパスでのメモリ割り当てを避け、適切なキューのバリエーションを利用することで、レイテンシを大幅に削減できます。

### バッファプール

バッファの取得と返却は、それぞれ 1 つのゴルーチンが担当します。`alloc` が成功すると、空きリストに返却用の枠が 1 つできます。バッファを使い終えたら、`free` を 1 回だけ呼んでインデックスを返します。

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

### イベント集約

集約処理はサービスの稼働中、動作し続けます。各プロデューサーは、キューへの追加に成功するまで同じ測定値を保持します。

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

// 単一アグリゲーター（単一コンシューマー）
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

### バックプレッシャー処理

呼び出し側で定義する `func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxAttempts int) bool` では、初回の試行も `maxAttempts` に含まれます。上限がゼロの場合は直ちに `false` を返します。

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

### グレースフルシャットダウン

新しく初期化したキューからバッチを取り出すときは、まずすべてのプロデューサーの最後の `Enqueue()` 呼び出しが戻るのを待ちます。キューが対応していれば `Drain()` を呼びます。その後、1 つのコンシューマーが初めて `Dequeue()` を呼び、`ErrWouldBlock` が返るまで取り出しを続けます。

```go
// プロデューサーゴルーチンが終了
prodWg.Wait()

// キュー変数を Init で初期化した場合は、そのアドレスを q として使う。
if d, ok := any(q).(lfq.Drainer); ok {
	d.Drain()
}

// キューが空を返すまでデキューする。
for {
	item, err := q.Dequeue()
	if err != nil {
		break
	}
	process(item)
}
```

FAA を使うジェネリック MPMC キューと SPMC キューでは、`Drain()` が取り出し時の閾値を無効にします。FAA を使う MPSC キューでは生産の終了を通知します。`MPMCIndirect` と `MPMCPtr` では `Drain()` を呼んでも何も起きません。SPSC キューからは直接取り出せます。

## キュー選択ガイド

```
┌─────────────────────────────────────────────────────────────────┐
│                      プロデューサーの数は？                     │
│                                                                 │
│      ┌──────────────────┐          ┌────────────────────┐       │
│      │    1つ (SPSC/    │          │    複数 (MPMC/     │       │
│      │    SPMC)         │          │    MPSC)           │       │
│      └────────┬─────────┘          └─────────┬──────────┘       │
│               │                               │                 │
│               ▼                               ▼                 │
│   ┌──────────────────┐              ┌──────────────────┐        │
│   │ コンシューマー1つ？│              │ コンシューマー1つ？│        │
│   └────────┬─────────┘              └────────┬─────────┘        │
│    はい    │   いいえ                はい    │   いいえ         │
│     │      │      │                   │      │      │           │
│     ▼      │      ▼                   ▼      │      ▼           │
│   SPSC     │    SPMC                MPSC     │    MPMC          │
│            │                                 │                  │
└────────────┴─────────────────────────────────┴──────────────────┘

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

最小容量は `2` です。`capacity < 2` の場合、コンストラクタは panic します。

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

Go の競合検出器では、`atomix` がアセンブリで行う同期の一部を追跡できません。`go test -race ./...` で競合検出を有効にしたテストを実行します。`go test ./...` を実行すると、競合検出時に省かれる並行テストと使用例も検証できます。

## 依存関係

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — セマンティックエラー (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — 明示的メモリオーダリングのアトミックプリミティブ
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — スピンプリミティブ

## プラットフォームサポート

| プラットフォーム | ステータス |
|------------------|-----------|
| linux/amd64, linux/arm64 (LSE2) | Linux 6.18 以降での本番運用 |
| linux/riscv64, linux/loong64 | CI でクロスコンパイル |
| darwin/amd64, darwin/arm64 | 開発用。CI では darwin/arm64 でテストを実行。 |
| freebsd/amd64, freebsd/arm64 | 開発用 |

## 参考文献

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *DISC 2019 (LIPIcs)*. https://doi.org/10.4230/LIPIcs.DISC.2019.28. Preprint: https://arxiv.org/abs/1908.04511.
- Lamport, L. (1977). Proving the Correctness of Multiprocess Programs. *IEEE Transactions on Software Engineering*, 3(2), 125–143.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## ライセンス

MIT — [LICENSE](./LICENSE) を参照。

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
