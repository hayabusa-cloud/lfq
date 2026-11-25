# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Benchmarks](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Idiomas:** [English](README.md) | [简体中文](README.zh-CN.md) | Español | [日本語](README.ja.md) | [Français](README.fr.md)

Implementaciones de colas FIFO sin bloqueo y sin espera para Go.

## Descripción General

El paquete `lfq` proporciona colas FIFO limitadas libres de bloqueos (lock-free) y libres de esperas (wait-free), optimizadas para diferentes patrones de productor/consumidor (SPSC, MPSC, SPMC, MPMC), garantizando un escalado predecible y sin asignaciones (zero-allocation) bajo alta contención.

### Teoría y Antecedentes

`lfq` se basa en investigaciones fundamentales de concurrencia para superar las limitaciones de los mutex tradicionales y los bucles CAS (Compare-And-Swap):

- **Buffer Circular de Lamport (1977)**: Impulsa nuestras rutas de Productor Único/Consumidor Único (SPSC), ofreciendo una latencia estrictamente de $O(1)$ libre de esperas sin costosas instrucciones atómicas de lectura-modificación-escritura.
- **Scalable Circular Queue (SCQ, 2019)**. El algoritmo SCQ de Ruslan Nikolaev es la base de las colas FAA genéricas.

### Los Stacks de I/O No Bloqueantes de `iox`

Cuando una operación devuelve `ErrWouldBlock`, el bucle de eventos puede atender otras tareas y reintentar más tarde. Quien llama de forma síncrona puede esperar con `iox.Backoff`.

```go
// Constructor directo (recomendado para la mayoría de casos)
q := lfq.NewSPSC[Event](1024)

// Builder API - selecciona automáticamente el algoritmo según restricciones
q := lfq.Build[Event](lfq.New(1024).SingleProducer().SingleConsumer())  // → SPSC
q := lfq.Build[Event](lfq.New(1024).SingleConsumer())                   // → MPSC
q := lfq.Build[Event](lfq.New(1024).SingleProducer().Compact())         // → SPMCSeq
q := lfq.Build[Event](lfq.New(1024).Compact())                          // → MPMCSeq
```

## Instalación

```bash
go get code.hybscloud.com/lfq
```

**Requisitos:** Go 1.27+

### Requisito del Compilador

Usa el compilador Atomix Go para las compilaciones de producción y las pruebas de rendimiento. [Atomix Go](https://github.com/hayabusa-cloud/go)

```bash
# Usando Makefile (recomendado)
make install-compiler   # Linux amd64/arm64 o macOS arm64
make build              # Compilar con el compilador de intrínsecos
make test               # Probar con el compilador de intrínsecos

# O compilar desde código fuente
make install-compiler-source
```

Ambos objetivos de instalación usan por defecto la última versión publicada del compilador Atomix para Go 1.27.x. Exporta `COMPILER_TAG` para seleccionar una versión concreta. La búsqueda de versiones usa `jq` y admite la autenticación en GitHub con `GH_TOKEN`. Para compilar desde las fuentes, usa la última versión oficial de Go 1.27.x en `PATH`.

Para la instalación manual, descargue el archivo para su plataforma y verifíquelo con el archivo `SHA256SUMS` de la misma versión. Coloque el directorio `go` extraído en `$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}`. [Versiones del compilador](https://github.com/hayabusa-cloud/go/releases)

```bash
# Compilar una aplicación que usa lfq
compiler="$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}"
GOTOOLCHAIN=local GOROOT="$compiler" "$compiler/bin/go" build ./...
```

El compilador Atomix integra en línea las operaciones de `atomix`. La CI ejecuta las pruebas de corrección con el compilador estándar de Go.

## Pruebas de rendimiento

La CI ejecuta `make bench-ci` en máquinas Linux amd64 y arm64 nativas con el compilador Atomix y `GOMAXPROCS=2`. El resumen de Actions muestra la mediana de diez muestras para la transferencia y el reintento de sesión, un par de operaciones de inserción y extracción en cada uno de los cuatro tipos de cola y `SPMCIndirectSeq` con consumidores concurrentes. Los artefactos de la ejecución contienen las muestras originales.

`Handoff` transfiere un valor entre dos goroutines mediante la cola de datos SPSC de cuatro posiciones que usa `sess` o un canal de Go con búfer de la misma capacidad. Ambos usan `iox.Backoff` cuando la cola está llena o vacía. `ns/op` incluye el tiempo de espera y `waits/op` cuenta las esperas de ambos extremos por valor entregado.

`FullRetry` mide en una sola goroutine un intento fallido de insertar en una cola llena, una extracción y un nuevo intento de inserción que tiene éxito. Ejecuta `make bench-session` para medir los casos de sesión o `make bench` para ejecutar todos los benchmarks.

## Tipos de Cola

| Tipo | Patrón | Garantía de Progreso | Caso de Uso |
|------|--------|---------------------|-------------|
| **SPSC** | Productor Único Consumidor Único | Sin espera | Etapas de pipeline, canales |
| **MPSC** | Múltiples Productores Consumidor Único | Sin bloqueo (dequeue sin espera) | Agregación de eventos, logging |
| **SPMC** | Productor Único Múltiples Consumidores | Sin bloqueo (enqueue sin espera) | Distribución de trabajo |
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

Las colas de acceso múltiple predeterminadas reservan posiciones mediante fetch-and-add (FAA) en un anillo de `2n` ranuras. En `MPMCIndirect` y `MPMCPtr`, los participantes publican y retiran los valores en posiciones consecutivas. Si quien reservó la siguiente posición se detiene, otro participante puede utilizarla.

- **Compensación**: Requiere `2n` slots físicos para una capacidad nominal `n`.
- **Capacidad transitoria**: Con `P` productores concurrentes, hasta `P-1` elementos adicionales pueden encolarse transitoriamente por encima de `Cap()` antes de que aplique la contrapresión.

En `MPSC[T]`, `MPSCIndirect` y `MPSCPtr` puede haber como máximo `Cap()` llamadas a `Enqueue` en curso, incluidas las de productores en pausa.

Al compartir `MPMC[T]`, `SPMC[T]`, `SPMCIndirect` o `SPMCPtr`, use una cola recién inicializada para un solo lote de hasta `Cap()` elementos y siga el [protocolo de vaciado con un único consumidor](#apagado-elegante). Use la cola `Seq` correspondiente si los consumidores consultan la cola durante la producción o extraen elementos en paralelo.

```go
// Múltiples productores, consumidor único
q := lfq.NewMPSC[Event](1024)  // Productores FAA, dequeue sin espera

// Un productor llena un lote en una cola nueva
q := lfq.NewSPMC[Task](1024)

// Varios productores llenan un lote en una cola nueva
q := lfq.NewMPMC[*Request](4096)
```

La validación de slots basada en ciclos proporciona seguridad ABA sin contadores de época ni punteros de riesgo.

### Variantes Indirect/Ptr: Operaciones Atómicas de 128 bits

Las variantes de cola Indirect y Ptr (todas las variantes no SPSC excepto Compact Indirect) empaquetan el número de secuencia y la carga útil en una sola operación atómica de 128 bits. Esto reduce la contención de línea de caché y mejora el rendimiento bajo alta concurrencia.

Para las variantes Ptr que codifican punteros en ranuras de 128 bits (FAA y PtrSeq), conserva una referencia tipada a cada objeto Go y mantén estable su dirección hasta que el consumidor asuma su propiedad.

```go
// Indirect guarda la secuencia y el valor en un par atómico.
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

// MPMC - opciones predeterminadas del constructor
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

// Puntero a un elemento de un array de mensajes declarado a nivel de paquete
q := lfq.NewMPMCPtr(len(messages))
q.Enqueue(unsafe.Pointer(&messages[i]))
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

Las variantes SPSC ya usan n slots (búfer circular de Lamport) e ignoran `Compact()`. En las demás topologías, `Compact().BuildIndirect()` admite valores de 63 bits. `SPMCCompactIndirect` y `MPMCCompactIndirect` también requieren que los valores sean distintos mientras varios consumidores puedan observarlos.

Para usar valores `uintptr` repetidos con varios consumidores, use `NewSPMCIndirectSeq` con un productor o `NewMPMCIndirect` con varios productores.

## Operaciones

| Operación | Retorna | Descripción |
|-----------|---------|-------------|
| `Enqueue(elem)` | `error` | Añadir elemento; retorna `ErrWouldBlock` si está llena |
| `Dequeue()` | `(T, error)` | Eliminar elemento; retorna `ErrWouldBlock` si está vacía |
| `Cap()` | `int` | Capacidad de la cola |

En `MPSCSeq[T]` y `MPMCSeq[T]`, `Dequeue` puede devolver `ErrWouldBlock` mientras un productor publica el siguiente elemento, aunque otro productor ya haya insertado un elemento posterior. En `SPMCSeq[T]` y `MPMCSeq[T]`, `Enqueue` puede devolverlo mientras un consumidor libera la siguiente posición, aunque otro consumidor ya haya retirado un elemento posterior.

### Manejo de Errores

```go
err := q.Enqueue(&item)
if lfq.IsWouldBlock(err) {
    // Conservar el elemento para volver a intentar la inserción.
}

elem, err := q.Dequeue()
if lfq.IsWouldBlock(err) {
    // Volver a intentar la extracción más tarde.
}
```

## Patrones de Uso

Los siguientes patrones demuestran cómo se puede integrar `lfq` en sistemas concurrentes. Al evitar asignaciones en la ruta crítica y utilizar las variantes de cola adecuadas, puede lograr reducciones sustanciales de latencia.

### Pool de Búferes

Una sola goroutine toma búferes del pool y una sola los devuelve. Cada llamada a `alloc` que tiene éxito deja sitio en la lista libre. Tras el último uso del búfer, llama a `free` exactamente una vez para devolver su índice.

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

### Agregación de Eventos

El agregador funciona mientras el servicio está activo. Cada productor conserva la lectura actual hasta que la cola la acepta.

```go
type Event struct {
	Source    string
	Timestamp time.Time
	Data      any
}

// Múltiples fuentes → Procesador único
events := lfq.NewMPSC[Event](max(8192, len(sensors)))

// Fuentes de eventos (múltiples productores)
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

// Agregador único (consumidor único)
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

### Manejo de Contrapresión

En la función auxiliar `func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxAttempts int) bool`, definida por el llamador, el primer intento cuenta para el límite. Si el límite es cero, la función devuelve `false` de inmediato.

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

### Apagado Elegante

Para vaciar un lote en una cola recién inicializada, espere a que termine la última llamada a `Enqueue()` de cada productor. Llame a `Drain()` si la cola lo admite. Un único consumidor hace entonces la primera llamada a `Dequeue()` y sigue extrayendo elementos hasta recibir `ErrWouldBlock`.

```go
// Las goroutines productoras terminan
prodWg.Wait()

// Si inicializó la cola con Init, use su dirección como q.
if d, ok := any(q).(lfq.Drainer); ok {
	d.Drain()
}

// Extraer hasta que la cola indique que está vacía.
for {
	item, err := q.Dequeue()
	if err != nil {
		break
	}
	process(item)
}
```

En las variantes basadas en FAA, `Drain()` desactiva el umbral de extracción de la cola MPMC genérica y de las colas SPMC. En las variantes MPSC basadas en FAA señala el fin de la producción. En `MPMCIndirect` y `MPMCPtr`, llamar a `Drain()` no tiene efecto. Las colas SPSC se vacían directamente.

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
│  SPSC[T]   │  SPMCSeq[T]           MPSC[T]   │  MPMCSeq[T]     │
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

La capacidad mínima es `2`. Los constructores hacen panic si `capacity < 2`.

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

El detector de carreras de Go no puede observar toda la sincronización implementada en ensamblador en `atomix`. Ejecuta `go test -race ./...` para las pruebas instrumentadas. Ejecuta `go test ./...` para incluir las pruebas concurrentes y los ejemplos que se omiten al activar el detector.

## Dependencias

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Errores semánticos (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — Primitivas atómicas con ordenamiento de memoria explícito
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — Primitivas de spin

## Soporte de Plataformas

| Plataforma | Estado |
|------------|--------|
| linux/amd64, linux/arm64 (LSE2) | Producción en Linux 6.18+ |
| linux/riscv64, linux/loong64 | Compilación cruzada en CI |
| darwin/amd64, darwin/arm64 | Desarrollo. La CI ejecuta pruebas en darwin/arm64. |
| freebsd/amd64, freebsd/arm64 | Desarrollo |

## Referencias

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *DISC 2019 (LIPIcs)*. https://doi.org/10.4230/LIPIcs.DISC.2019.28. Preprint: https://arxiv.org/abs/1908.04511.
- Lamport, L. (1977). Proving the Correctness of Multiprocess Programs. *IEEE Transactions on Software Engineering*, 3(2), 125–143.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## Licencia

MIT — ver [LICENSE](./LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
