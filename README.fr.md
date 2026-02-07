# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/lfq)](https://goreportcard.com/report/github.com/hayabusa-cloud/lfq)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Langues:** [English](README.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md) | [Español](README.es.md) | Français

Implémentations de files FIFO sans verrou et sans attente pour Go.

## Aperçu

lfq fournit des files FIFO bornées optimisées pour différents modèles producteur/consommateur. Chaque variante utilise l'algorithme approprié pour son modèle d'accès.

```go
// Constructeur direct (recommandé pour la plupart des cas)
q := lfq.NewSPSC[Event](1024)

// Builder API - sélectionne automatiquement l'algorithme selon les contraintes
q := lfq.Build[Event](lfq.New(1024).SingleProducer().SingleConsumer())  // → SPSC
q := lfq.Build[Event](lfq.New(1024).SingleConsumer())                   // → MPSC
q := lfq.Build[Event](lfq.New(1024).SingleProducer())                   // → SPMC
q := lfq.Build[Event](lfq.New(1024))                                    // → MPMC
```

## Installation

```bash
go get code.hybscloud.com/lfq
```

**Prérequis:** Go 1.25+

### Exigence du Compilateur

Pour de meilleures performances, compilez avec le [compilateur Go optimisé avec intrinsèques](https://github.com/hayabusa-cloud/go) :

```bash
# Utilisation du Makefile (recommandé)
make install-compiler   # Télécharger la version pré-compilée (~30 secondes)
make build              # Compiler avec le compilateur d'intrinsèques
make test               # Tester avec le compilateur d'intrinsèques

# Ou compiler depuis les sources (dernière version de développement)
make install-compiler-source
```

Installation manuelle :

```bash
# Version pré-compilée (recommandée)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
URL=$(curl -fsSL https://api.github.com/repos/hayabusa-cloud/go/releases/latest | grep "browser_download_url.*${OS}-${ARCH}\.tar\.gz\"" | cut -d'"' -f4)
curl -fsSL "$URL" | tar -xz -C ~/sdk
mv ~/sdk/go ~/sdk/go-atomix

# Utiliser pour compiler le code dépendant de lfq
GOROOT=~/sdk/go-atomix ~/sdk/go-atomix/bin/go build ./...
```

Le compilateur d'intrinsèques intègre les opérations atomix avec l'ordonnancement mémoire correct. Le compilateur Go standard fonctionne pour les tests de base mais peut présenter des problèmes sous haute contention.

## Types de Files

| Type | Modèle | Garantie de Progrès | Cas d'Utilisation |
|------|--------|---------------------|-------------------|
| **SPSC** | Producteur Unique Consommateur Unique | Sans attente | Étapes de pipeline, canaux |
| **MPSC** | Producteurs Multiples Consommateur Unique | Sans verrou | Agrégation d'événements, logging |
| **SPMC** | Producteur Unique Consommateurs Multiples | Sans verrou | Distribution de travail |
| **MPMC** | Producteurs Multiples Consommateurs Multiples | Sans verrou | Usage général |

### Garanties de Progrès

- **Sans attente (Wait-free)**: Chaque opération se termine en étapes bornées (O(1))
- **Sans verrou (Lock-free)**: Progrès garanti au niveau système; au moins un thread progresse

## Algorithmes

### SPSC: Buffer Circulaire de Lamport

Buffer borné classique avec optimisation d'index en cache.

```go
q := lfq.NewSPSC[int](1024)

// Producteur
q.Enqueue(&value)  // Sans attente O(1)

// Consommateur
elem, err := q.Dequeue()  // Sans attente O(1)
```

### MPSC/SPMC/MPMC: Basé sur FAA (Par Défaut)

Par défaut, les files à accès multiple utilisent des algorithmes basés sur FAA (Fetch-And-Add) dérivés de SCQ (File Circulaire Évolutive). FAA incrémente aveuglément les compteurs de position, nécessitant 2n emplacements physiques pour une capacité n, mais offre une meilleure évolutivité sous haute contention que les alternatives basées sur CAS.

```go
// Producteurs multiples, consommateur unique
q := lfq.NewMPSC[Event](1024)  // Producteurs FAA, dequeue sans attente

// Producteur unique, consommateurs multiples
q := lfq.NewSPMC[Task](1024)   // Enqueue sans attente, consommateurs FAA

// Producteurs et consommateurs multiples
q := lfq.NewMPMC[*Request](4096)  // Algorithme SCQ basé sur FAA
```

La validation d'emplacements basée sur les cycles fournit la sécurité ABA sans compteurs d'époque ni pointeurs de danger.

### Variantes Indirect/Ptr: Opérations Atomiques 128 bits

Les variantes de file Indirect et Ptr (non SPSC, non Compact) empaquettent le numéro de séquence et la valeur dans une seule opération atomique de 128 bits. Cela réduit la contention de ligne de cache et améliore le débit sous haute concurrence.

```go
// Indirect - une opération atomique de 128 bits par opération
q := lfq.NewMPMCIndirect(4096)

// Ptr - même optimisation pour unsafe.Pointer
q := lfq.NewMPMCPtr(4096)
```

## Builder API

Sélection automatique d'algorithme basée sur les contraintes:

```go
// SPSC - les deux contraintes → anneau de Lamport
q := lfq.Build[T](lfq.New(1024).SingleProducer().SingleConsumer())

// MPSC - consommateur unique seulement
q := lfq.Build[T](lfq.New(1024).SingleConsumer())

// SPMC - producteur unique seulement
q := lfq.Build[T](lfq.New(1024).SingleProducer())

// MPMC - sans contraintes (par défaut)
q := lfq.Build[T](lfq.New(1024))
```

## Variantes

Chaque type de file a trois variantes:

| Variante | Type d'Élément | Cas d'Utilisation |
|----------|---------------|-------------------|
| Generic | `[T any]` | Typage sûr, usage général |
| Indirect | `uintptr` | Pools basés sur index, handles |
| Ptr | `unsafe.Pointer` | Passage de pointeurs sans copie |

```go
// Générique
q := lfq.NewMPMC[MyStruct](1024)

// Indirect - pour indices de pool
q := lfq.NewMPMCIndirect(1024)
q.Enqueue(uintptr(poolIndex))

// Pointeur - sans copie
q := lfq.NewMPMCPtr(1024)
q.Enqueue(unsafe.Pointer(obj))
```

### Mode Compact

Compact() sélectionne les algorithmes basés sur CAS qui utilisent n emplacements physiques (contre 2n pour le défaut basé sur FAA). Utilisez quand l'efficacité mémoire est plus importante que l'évolutivité sous contention :

```go
// Mode compact - basé sur CAS, n emplacements
q := lfq.New(4096).Compact().BuildIndirect()
```

| Mode | Algorithme | Emplacements Physiques | Utilisation |
|------|------------|------------------------|-------------|
| Par défaut | Basé sur FAA | 2n | Haute contention, évolutivité |
| Compact | Basé sur CAS | n | Mémoire limitée |

Les variantes SPSC utilisent déjà n emplacements (buffer circulaire de Lamport) et ignorent Compact(). Pour les files Indirect avec Compact(), les valeurs sont limitées à 63 bits.

## Opérations

| Opération | Retour | Description |
|-----------|--------|-------------|
| `Enqueue(elem)` | `error` | Ajouter élément; retourne `ErrWouldBlock` si pleine |
| `Dequeue()` | `(T, error)` | Retirer élément; retourne `ErrWouldBlock` si vide |
| `Cap()` | `int` | Capacité de la file |

### Gestion des Erreurs

```go
err := q.Enqueue(&item)
if lfq.IsWouldBlock(err) {
    // File pleine - appliquer la contre-pression ou réessayer
}

elem, err := q.Dequeue()
if lfq.IsWouldBlock(err) {
    // File vide - attendre ou sonder
}
```

## Modèles d'Utilisation

### Pool de Tampons

```go
const poolSize = 1024
const bufSize = 4096

// Pré-allouer les tampons
pool := make([][]byte, poolSize)
for i := range pool {
    pool[i] = make([]byte, bufSize)
}

// La liste libre suit les indices disponibles
freeList := lfq.NewSPSCIndirect(poolSize)
for i := range poolSize {
    freeList.Enqueue(uintptr(i))
}

// Allouer
func Alloc() ([]byte, uintptr, bool) {
    idx, err := freeList.Dequeue()
    if err != nil {
        return nil, 0, false
    }
    return pool[idx], idx, true
}

// Libérer
func Free(idx uintptr) {
    freeList.Enqueue(idx)
}
```

### Agrégation d'Événements

```go
type Event struct {
    Source    string
    Timestamp time.Time
    Data      any
}

// Sources multiples → Processeur unique
events := lfq.NewMPSC[Event](8192)

// Sources d'événements (producteurs multiples)
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

// Agrégateur unique (consommateur unique)
go func() {
    for {
        ev, err := events.Dequeue()
        if err == nil {
            aggregate(*ev)
        }
    }
}()
```

### Gestion de la Contre-pression

```go
// Enqueue avec réessai et yield
func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxRetries int) bool {
	ba := iox.Backoff{}
    for i := range maxRetries {
        if q.Enqueue(&item) == nil {
            return true
        }
        ba.Wait() // Céder pour permettre aux consommateurs de drainer
    }
    return false // Appliquer la contre-pression à l'appelant
}

```

### Arrêt Gracieux

Les files basées sur FAA (MPMC, SPMC, MPSC) incluent un mécanisme de seuil pour prévenir le livelock. Pour un arrêt gracieux où les producteurs terminent avant les consommateurs, utilisez l'interface `Drainer` :

```go
// Les goroutines productrices terminent
prodWg.Wait()

// Signaler qu'il n'y aura plus d'enqueues
if d, ok := q.(lfq.Drainer); ok {
    d.Drain()
}

// Les consommateurs peuvent maintenant drainer tous les éléments
// restants sans blocage par seuil
for {
    item, err := q.Dequeue()
    if err != nil {
        break // La file est vide
    }
    process(item)
}
```

`Drain()` est un indice — l'appelant doit s'assurer qu'aucun autre appel à `Enqueue()` ne sera fait. Les files SPSC n'implémentent pas `Drainer` car elles n'ont pas de mécanisme de seuil ; l'assertion de type gère ce cas naturellement.

## Quand Utiliser Quelle File

```
┌─────────────────────────────────────────────────────────────────┐
│                    Combien de producteurs ?                      │
│                                                                 │
│      ┌──────────────────┐          ┌────────────────────┐      │
│      │    Un (SPSC/      │          │   Multiples (MPMC/ │      │
│      │    SPMC)          │          │   MPSC)            │      │
│      └────────┬─────────┘          └─────────┬──────────┘      │
│               │                               │                 │
│               ▼                               ▼                 │
│   ┌──────────────────┐              ┌──────────────────┐       │
│   │ Un consommateur? │              │ Un consommateur? │       │
│   └────────┬─────────┘              └────────┬─────────┘       │
│    Oui     │     Non                 Oui     │     Non         │
│     │      │      │                   │      │      │          │
│     ▼      │      ▼                   ▼      │      ▼          │
│   SPSC     │    SPMC                MPSC     │    MPMC         │
│            │                                 │                  │
└────────────┴─────────────────────────────────┴─────────────────┘

Sélection de Variante :
• Generic [T]     → Typage sûr, sémantique de copie
• Indirect        → Index de pool, offsets de tampon (uintptr)
• Ptr             → Passage d'objets sans copie (unsafe.Pointer)
```

### Capacité

La capacité est arrondie à la puissance de 2 suivante :

```go
q := lfq.NewMPMC[int](3)     // Capacité réelle : 4
q := lfq.NewMPMC[int](4)     // Capacité réelle : 4
q := lfq.NewMPMC[int](1000)  // Capacité réelle : 1024
q := lfq.NewMPMC[int](1024)  // Capacité réelle : 1024
```

## Disposition Mémoire

Toutes les files utilisent un remplissage de ligne de cache (64 octets) pour éviter le faux partage :

```go
type MPMC[T any] struct {
    _        [64]byte      // Remplissage
    tail     atomix.Uint64 // Index du producteur
    _        [64]byte      // Remplissage
    head     atomix.Uint64 // Index du consommateur
    _        [64]byte      // Remplissage
    buffer   []slot[T]
    // ...
}
```

## Détection de Conditions de Course

Le détecteur de courses de Go n'est pas conçu pour vérifier les algorithmes lock-free. Il suit les primitives de synchronisation explicites (mutex, canaux) mais ne peut pas observer les relations happens-before établies par l'ordonnancement mémoire atomique.

Les tests utilisent deux mécanismes de protection :
- Balise de compilation `//go:build !race` exclut les fichiers d'exemple des tests de course
- Vérification à l'exécution `if lfq.RaceEnabled { t.Skip() }` ignore les tests concurrents dans `lockfree_test.go`

Exécutez `go test -race ./...` pour les tests sûrs, ou `go test ./...` pour tous les tests.

## Dépendances

- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Erreurs sémantiques (`ErrWouldBlock`)
- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — Primitives atomiques avec ordonnancement mémoire explicite
- [code.hybscloud.com/spin](https://code.hybscloud.com/spin) — Primitives de spin

## Support des Plateformes

| Plateforme | Statut |
|------------|--------|
| linux/amd64 | Principal |
| linux/arm64 | Supporté |
| linux/riscv64 | Supporté |
| linux/loong64 | Supporté |
| darwin/amd64, darwin/arm64 | Supporté |
| freebsd/amd64, freebsd/arm64 | Supporté |

## Références

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *arXiv*, arXiv:1908.04511. https://arxiv.org/abs/1908.04511.
- Lamport, L. (1974). A New Solution of Dijkstra's Concurrent Programming Problem. *Communications of the ACM*, 17(8), 453–455.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## Licence

MIT — voir [LICENSE](./LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
