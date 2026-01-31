# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/lfq)](https://goreportcard.com/report/github.com/hayabusa-cloud/lfq)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Idiomas:** [English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | Español | [Français](README.fr.md)

Implementaciones de colas FIFO sin bloqueo y sin espera para Go.

## Descripción General

lfq proporciona colas FIFO acotadas optimizadas para diferentes patrones de productor/consumidor. Cada variante utiliza el algoritmo adecuado para su patrón de acceso.

```go
// Constructor directo (recomendado para la mayoría de casos)
q := lfq.NewSPSC[Event](1024)

// Builder API - selecciona automáticamente el algoritmo según restricciones
q := lfq.Build[Event](lfq.New(1024).SingleProducer().SingleConsumer())  // → SPSC
q := lfq.Build[Event](lfq.New(1024).SingleConsumer())                   // → MPSC
q := lfq.Build[Event](lfq.New(1024).SingleProducer())                   // → SPMC
q := lfq.Build[Event](lfq.New(1024))                                    // → MPMC
```

## Instalación

```bash
go get code.hybscloud.com/lfq
```

**Requisitos:** Go 1.25+

### Requisito del Compilador

Para un mejor rendimiento, compile con el [compilador Go optimizado con intrínsecos](https://github.com/hayabusa-cloud/go):

```bash
# Usando Makefile (recomendado)
make install-compiler   # Descargar versión pre-compilada (~30 segundos)
make build              # Compilar con el compilador de intrínsecos
make test               # Probar con el compilador de intrínsecos

# O compilar desde código fuente (última versión de desarrollo)
make install-compiler-source
```

Instalación manual:

```bash
# Versión pre-compilada (recomendada)
curl -fsSL https://github.com/hayabusa-cloud/go/releases/latest/download/go1.25.6.linux-amd64.tar.gz | tar -xz -C ~/sdk
mv ~/sdk/go ~/sdk/go-atomix

# Usar para compilar código dependiente de lfq
GOROOT=~/sdk/go-atomix ~/sdk/go-atomix/bin/go build ./...
```

El compilador de intrínsecos incorpora las operaciones de atomix con el ordenamiento de memoria correcto. El compilador Go estándar funciona para pruebas básicas pero puede presentar problemas bajo alta contención.

## Tipos de Cola

| Tipo | Patrón | Garantía de Progreso | Caso de Uso |
|------|--------|---------------------|-------------|
| **SPSC** | Productor Único Consumidor Único | Sin espera | Etapas de pipeline, canales |
| **MPSC** | Múltiples Productores Consumidor Único | Sin bloqueo | Agregación de eventos, logging |
| **SPMC** | Productor Único Múltiples Consumidores | Sin bloqueo | Distribución de trabajo |
| **MPMC** | Múltiples Productores Múltiples Consumidores | Sin bloqueo | Propósito general |

### Garantías de Progreso

- **Sin espera (Wait-free)**: Cada operación se completa en pasos acotados (O(1))
- **Sin bloqueo (Lock-free)**: Progreso garantizado a nivel de sistema; al menos un hilo avanza

## Algoritmos

### SPSC: Buffer Circular de Lamport

Buffer acotado clásico con optimización de índices en caché.

```go
q := lfq.NewSPSC[int](1024)

// Productor
q.Enqueue(&value)  // Sin espera O(1)

// Consumidor
elem, err := q.Dequeue()  // Sin espera O(1)
```

### MPSC/SPMC/MPMC: Basado en FAA (Predeterminado)

Por defecto, las colas de acceso múltiple usan algoritmos basados en FAA (Fetch-And-Add) derivados de SCQ (Cola Circular Escalable). FAA incrementa ciegamente los contadores de posición, requiriendo 2n slots físicos para capacidad n, pero escala mejor bajo alta contención que las alternativas basadas en CAS.

```go
// Múltiples productores, consumidor único
q := lfq.NewMPSC[Event](1024)  // Productores FAA, dequeue sin espera

// Productor único, múltiples consumidores
q := lfq.NewSPMC[Task](1024)   // Enqueue sin espera, consumidores FAA

// Múltiples productores y consumidores
q := lfq.NewMPMC[*Request](4096)  // Algoritmo SCQ basado en FAA
```

La validación de slots basada en ciclos proporciona seguridad ABA sin contadores de época ni punteros de riesgo.

### Variantes Indirect/Ptr: Operaciones Atómicas de 128 bits

Las variantes de cola Indirect y Ptr (no SPSC, no Compact) empaquetan el número de secuencia y el valor en una sola operación atómica de 128 bits. Esto reduce la contención de línea de caché y mejora el rendimiento bajo alta concurrencia.

```go
// Indirect - una operación atómica de 128 bits por operación
q := lfq.NewMPMCIndirect(4096)

// Ptr - misma optimización para unsafe.Pointer
q := lfq.NewMPMCPtr(4096)
```

## Builder API

Selección automática de algoritmo basada en restricciones:

```go
// SPSC - ambas restricciones → anillo de Lamport
q := lfq.Build[T](lfq.New(1024).SingleProducer().SingleConsumer())

// MPSC - solo consumidor único
q := lfq.Build[T](lfq.New(1024).SingleConsumer())

// SPMC - solo productor único
q := lfq.Build[T](lfq.New(1024).SingleProducer())

// MPMC - sin restricciones (predeterminado)
q := lfq.Build[T](lfq.New(1024))
```

## Variantes

Cada tipo de cola tiene tres variantes:

| Variante | Tipo de Elemento | Caso de Uso |
|----------|-----------------|-------------|
| Generic | `[T any]` | Seguro en tipos, propósito general |
| Indirect | `uintptr` | Pools basados en índices, handles |
| Ptr | `unsafe.Pointer` | Paso de punteros sin copia |

```go
// Genérico
q := lfq.NewMPMC[MyStruct](1024)

// Indirecto - para índices de pool
q := lfq.NewMPMCIndirect(1024)
q.Enqueue(uintptr(poolIndex))

// Puntero - sin copia
q := lfq.NewMPMCPtr(1024)
q.Enqueue(unsafe.Pointer(obj))
```

### Modo Compacto

Compact() selecciona algoritmos basados en CAS que usan n slots físicos (vs 2n para el predeterminado basado en FAA). Use cuando la eficiencia de memoria es más importante que la escalabilidad bajo contención:

```go
// Modo compacto - basado en CAS, n slots
q := lfq.New(4096).Compact().BuildIndirect()
```

| Modo | Algoritmo | Slots Físicos | Uso |
|------|-----------|---------------|-----|
| Predeterminado | Basado en FAA | 2n | Alta contención, escalabilidad |
| Compacto | Basado en CAS | n | Memoria limitada |

Las variantes SPSC ya usan n slots (buffer circular de Lamport) e ignoran Compact(). Para colas Indirect con Compact(), los valores están limitados a 63 bits.

## Operaciones

| Operación | Retorna | Descripción |
|-----------|---------|-------------|
| `Enqueue(elem)` | `error` | Añadir elemento; retorna `ErrWouldBlock` si está llena |
| `Dequeue()` | `(T, error)` | Eliminar elemento; retorna `ErrWouldBlock` si está vacía |
| `Cap()` | `int` | Capacidad de la cola |

### Manejo de Errores

```go
err := q.Enqueue(&item)
if lfq.IsWouldBlock(err) {
    // Cola llena - aplicar contrapresión o reintentar
}

elem, err := q.Dequeue()
if lfq.IsWouldBlock(err) {
    // Cola vacía - esperar o sondear
}
```

## Patrones de Uso

### Pool de Buffers

```go
const poolSize = 1024
const bufSize = 4096

// Pre-asignar buffers
pool := make([][]byte, poolSize)
for i := range pool {
    pool[i] = make([]byte, bufSize)
}

// Lista libre rastrea índices disponibles
freeList := lfq.NewSPSCIndirect(poolSize)
for i := range poolSize {
    freeList.Enqueue(uintptr(i))
}

// Asignar
func Alloc() ([]byte, uintptr, bool) {
    idx, err := freeList.Dequeue()
    if err != nil {
        return nil, 0, false
    }
    return pool[idx], idx, true
}

// Liberar
func Free(idx uintptr) {
    freeList.Enqueue(idx)
}
```

### Agregación de Eventos

```go
type Event struct {
    Source    string
    Timestamp time.Time
    Data      any
}

// Múltiples fuentes → Procesador único
events := lfq.NewMPSC[Event](8192)

// Fuentes de eventos (múltiples productores)
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

// Agregador único (consumidor único)
go func() {
    for {
        ev, err := events.Dequeue()
        if err == nil {
            aggregate(*ev)
        }
    }
}()
```

### Manejo de Contrapresión

```go
// Enqueue con reintento y yield
func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxRetries int) bool {
	ba := iox.Backoff{}
    for i := range maxRetries {
        if q.Enqueue(&item) == nil {
            return true
        }
        ba.Wait() // Ceder para permitir que los consumidores drenen
    }
    return false // Aplicar contrapresión al llamador
}

```

## Cuándo Usar Cada Cola

```
┌─────────────────────────────────────────────────────────────────┐
│                    ¿Cuántos productores?                         │
│                                                                 │
│      ┌──────────────────┐          ┌────────────────────┐      │
│      │    Uno (SPSC/     │          │   Múltiples (MPMC/ │      │
│      │    SPMC)          │          │   MPSC)            │      │
│      └────────┬─────────┘          └─────────┬──────────┘      │
│               │                               │                 │
│               ▼                               ▼                 │
│   ┌──────────────────┐              ┌──────────────────┐       │
│   │ ¿Un consumidor?  │              │ ¿Un consumidor?  │       │
│   └────────┬─────────┘              └────────┬─────────┘       │
│    Sí      │     No                  Sí      │     No          │
│     │      │      │                   │      │      │          │
│     ▼      │      ▼                   ▼      │      ▼          │
│   SPSC     │    SPMC                MPSC     │    MPMC         │
│            │                                 │                  │
└────────────┴─────────────────────────────────┴─────────────────┘

Selección de Variante:
• Generic [T]     → Tipo seguro, semántica de copia
• Indirect        → Índices de pool, offsets de buffer (uintptr)
• Ptr             → Paso de objetos sin copia (unsafe.Pointer)
```

### Capacidad

La capacidad se redondea a la siguiente potencia de 2:

```go
q := lfq.NewMPMC[int](3)     // Capacidad real: 4
q := lfq.NewMPMC[int](4)     // Capacidad real: 4
q := lfq.NewMPMC[int](1000)  // Capacidad real: 1024
q := lfq.NewMPMC[int](1024)  // Capacidad real: 1024
```

## Diseño de Memoria

Todas las colas usan relleno de línea de caché (64 bytes) para prevenir el falso compartido:

```go
type MPMC[T any] struct {
    _        [64]byte      // Relleno
    tail     atomix.Uint64 // Índice del productor
    _        [64]byte      // Relleno
    head     atomix.Uint64 // Índice del consumidor
    _        [64]byte      // Relleno
    buffer   []slot[T]
    // ...
}
```

## Detección de Condiciones de Carrera

El detector de carreras de Go no está diseñado para verificar algoritmos lock-free. Rastrea primitivas de sincronización explícitas (mutex, canales) pero no puede observar las relaciones happens-before establecidas por el ordenamiento de memoria atómico.

Las pruebas usan dos mecanismos de protección:
- Etiqueta de compilación `//go:build !race` excluye archivos de ejemplo de las pruebas de carrera
- Verificación en tiempo de ejecución `if lfq.RaceEnabled { t.Skip() }` omite pruebas concurrentes en `lockfree_test.go`

Ejecute `go test -race ./...` para pruebas seguras ante carreras, o `go test ./...` para todas las pruebas.

## Dependencias

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Errores semánticos (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — Primitivas atómicas con ordenamiento de memoria explícito
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — Primitivas de spin

## Soporte de Plataformas

| Plataforma | Estado |
|------------|--------|
| linux/amd64 | Principal |
| linux/arm64 | Soportado |
| linux/riscv64 | Soportado |
| linux/loong64 | Soportado |
| darwin/amd64, darwin/arm64 | Soportado |
| freebsd/amd64, freebsd/arm64 | Soportado |

## Referencias

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *arXiv*, arXiv:1908.04511. https://arxiv.org/abs/1908.04511.
- Lamport, L. (1974). A New Solution of Dijkstra's Concurrent Programming Problem. *Communications of the ACM*, 17(8), 453–455.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## Licencia

MIT — ver [LICENSE](./LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
