# lfq

[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/lfq.svg)](https://pkg.go.dev/code.hybscloud.com/lfq)
[![Benchmarks](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/lfq/actions/workflows/benchmarks.yml)
[![Codecov](https://codecov.io/gh/hayabusa-cloud/lfq/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/lfq)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Langues:** [English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | Français

Implémentations de files FIFO sans verrou et sans attente pour Go.

## Aperçu

Le paquet `lfq` fournit des files d'attente FIFO bornées sans verrou (lock-free) et sans attente (wait-free), optimisées pour différents modèles de producteur/consommateur (SPSC, MPSC, SPMC, MPMC), garantissant une mise à l'échelle prévisible et sans allocation sous haute contention.

### Théorie et Contexte

`lfq` s'appuie sur la recherche fondamentale en concurrence pour surmonter les limites des mutex traditionnels et des boucles CAS (Compare-And-Swap) :

- **Tampon Circulaire de Lamport (1977)** : Alimente nos chemins SPSC (Single-Producer/Single-Consumer), offrant une latence strictement $O(1)$ sans attente, sans instructions atomiques de lecture-modification-écriture coûteuses.
- **Scalable Circular Queue (SCQ, 2019)** : Nos chemins multipartites (MPSC, SPMC, MPMC) sont implémentés sur la base de l'algorithme SCQ de Ruslan Nikolaev. En utilisant des instructions matérielles Fetch-And-Add (FAA) au lieu de boucles CAS, `lfq` évite de manière inhérente les goulots d'étranglement par contention et les problèmes ABA, fonctionnant comme une file d'attente autonome sans mécanismes externes de récupération sécurisée de mémoire (ex. hazard pointers).

### Les Stacks d'I/O Non Bloquantes d'`iox`

Lorsqu’une opération renvoie `ErrWouldBlock`, la boucle d’événements peut traiter d’autres tâches avant de réessayer. En mode synchrone, l’appelant peut attendre avec `iox.Backoff`.

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

**Prérequis:** Go 1.27+

### Exigence du Compilateur

Pour de meilleures performances, compilez avec le [compilateur Go optimisé avec intrinsèques](https://github.com/hayabusa-cloud/go) :

```bash
# Utilisation du Makefile (recommandé)
make install-compiler   # Linux amd64/arm64 ou macOS arm64
make build              # Compiler avec le compilateur d'intrinsèques
make test               # Tester avec le compilateur d'intrinsèques

# Ou compiler depuis les sources
make install-compiler-source
```

Les deux cibles d’installation utilisent par défaut la dernière version publiée du compilateur Atomix pour Go 1.27.x. Exportez `COMPILER_TAG` pour sélectionner une version précise. La recherche des versions nécessite `jq`. Pour compiler depuis les sources, utilisez la dernière version officielle de Go 1.27.x dans le `PATH`.

Pour une installation manuelle, téléchargez l’archive de votre plateforme et vérifiez-la à l’aide du fichier `SHA256SUMS` de la même version. Placez le répertoire `go` extrait dans `$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}`. [Versions du compilateur](https://github.com/hayabusa-cloud/go/releases)

```bash
# Compiler une application qui utilise lfq
compiler="$HOME/sdk/${COMPILER_TAG:-go1.27-atomix}"
GOTOOLCHAIN=local GOROOT="$compiler" "$compiler/bin/go" build ./...
```

Le compilateur d'intrinsèques intègre les opérations atomix avec l'ordonnancement mémoire correct. Le compilateur Go standard fonctionne pour les tests de base mais peut présenter des problèmes sous haute contention.

## Mesures de performance

Après avoir installé le compilateur, exécutez `make bench-session` pour obtenir dix échantillons avec `GOMAXPROCS=2`. `Handoff` transmet une valeur entre deux goroutines au moyen de la file de données SPSC à quatre places utilisée par `sess` ou d’un canal Go doté d’un tampon de même capacité. Les deux utilisent `iox.Backoff` quand la file est pleine ou vide. `ns/op` inclut le temps d’attente et `waits/op` compte les attentes des deux côtés pour chaque valeur transmise.

`FullRetry` mesure dans une seule goroutine une tentative d’insertion dans une file pleine, le retrait d’une valeur et la réussite d’une nouvelle tentative d’insertion. Exécutez `make bench` pour lancer tous les benchmarks.

## Types de Files

| Type | Modèle | Garantie de Progrès | Cas d'Utilisation |
|------|--------|---------------------|-------------------|
| **SPSC** | Producteur Unique Consommateur Unique | Sans attente | Étapes de pipeline, canaux |
| **MPSC** | Producteurs Multiples Consommateur Unique | Sans verrou (dequeue sans attente) | Agrégation d'événements, logging |
| **SPMC** | Producteur Unique Consommateurs Multiples | Sans verrou (enqueue sans attente) | Distribution de travail |
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

Par défaut, les files à accès multiple implémentent l'algorithme SCQ (File Circulaire Évolutive) en utilisant des instructions FAA (Fetch-And-Add). FAA incrémente aveuglément les compteurs de position, nécessitant 2n emplacements physiques pour une capacité n, mais offre une meilleure évolutivité sous haute contention que les alternatives basées sur CAS.

- **Compromis** : Nécessite `2n` emplacements physiques pour une capacité nominale `n`.
- **Capacité transitoire** : Avec `P` producteurs concurrents, jusqu'à `P-1` éléments supplémentaires peuvent être temporairement enfilés au-delà de `Cap()` avant l'application de la contre-pression.

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

Les variantes de file Indirect et Ptr (toutes les variantes non SPSC, sauf Compact Indirect) empaquettent le numéro de séquence et la charge utile dans une seule opération atomique de 128 bits. Cela réduit la contention de ligne de cache et améliore le débit sous haute concurrence.

Pour les variantes Ptr qui encodent des pointeurs dans des emplacements 128 bits (variantes FAA et PtrSeq), conservez une référence Go typée vers les objets enfilés jusqu'à leur défilement (ou jusqu'à consommation garantie). Ne vous appuyez pas uniquement sur les bits de pointeur stockés dans la file comme racine d'accessibilité du GC.

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

Les variantes SPSC utilisent déjà n emplacements (buffer circulaire de Lamport) et ignorent Compact(). Pour les files Indirect avec Compact(), les valeurs sont limitées à 63 bits. Pour `SPMCCompactIndirect` et `MPMCCompactIndirect`, l'appelant doit aussi garantir que les valeurs restent distinctes tant qu'elles peuvent être observées par des consommateurs concurrents; utilisez les files indirectes par défaut lorsque des valeurs `uintptr` dupliquées sont requises avec un accès multi-consommateur.

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

Les modèles suivants montrent comment `lfq` peut être intégré dans des systèmes concurrents. En évitant les allocations dans le chemin critique et en utilisant les variantes de file d'attente appropriées, vous pouvez obtenir des réductions de latence substantielles.

### Pool de Tampons

Une seule goroutine emprunte les tampons et une seule les rend. Chaque appel réussi à `alloc` libère une place dans la file. Après la dernière utilisation du tampon, appelez `free` une seule fois pour rendre son indice.

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

### Agrégation d'Événements

L’agrégateur fonctionne pendant toute la durée du service. Chaque producteur conserve sa mesure jusqu’à ce que la file l’accepte.

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

// Agrégateur unique (consommateur unique)
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

### Gestion de la Contre-pression

Dans la fonction auxiliaire `func EnqueueWithRetry(q lfq.Queue[Item], item Item, maxAttempts int) bool` définie par l’appelant, la limite inclut la première tentative. Si elle est nulle, la fonction renvoie immédiatement `false`.

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

### Arrêt Gracieux

Les files FAA à consommateurs multiples (MPMC, SPMC) incluent un mécanisme de seuil pour prévenir le livelock. MPSC implémente aussi `Drainer`, mais uniquement comme signal d'arrêt gracieux (pas de saut de seuil dans `Dequeue`). Pour un arrêt gracieux où les producteurs terminent avant les consommateurs, utilisez l'interface `Drainer` :

```go
// Les goroutines productrices terminent
prodWg.Wait()

// Signaler qu'il n'y aura plus d'enqueues
if d, ok := q.(lfq.Drainer); ok {
    d.Drain()
}

// Les consommateurs peuvent maintenant drainer tous les éléments restants.
// Pour MPMC/SPMC, Drain évite les sorties anticipées dues au seuil.
for {
    item, err := q.Dequeue()
    if err != nil {
        break // La file est vide
    }
    process(item)
}
```

`Drain()` est un indice — l'appelant doit s'assurer qu'aucun autre appel à `Enqueue()` ne sera fait. Pour MPMC/SPMC, `Drain()` évite les sorties anticipées dues au seuil dans `Dequeue`. Pour MPSC, `Drain()` est uniquement un signal d'arrêt. Les files SPSC n'implémentent pas `Drainer` ; l'assertion de type gère ce cas naturellement.

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

La capacité minimale est `2`. Les constructeurs panic si `capacity < 2`.

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

- Nikolaev, R. (2019). A Scalable, Portable, and Memory-Efficient Lock-Free FIFO Queue. *DISC 2019 (LIPIcs)*. https://doi.org/10.4230/LIPIcs.DISC.2019.28. Preprint: https://arxiv.org/abs/1908.04511.
- Lamport, L. (1977). Proving the Correctness of Multiprocess Programs. *IEEE Transactions on Software Engineering*, 3(2), 125–143.
- Vyukov, D. (2010). Bounded MPMC Queue. *1024cores.net*. https://1024cores.net/home/lock-free-algorithms/queues/bounded-mpmc-queue.
- Herlihy, M. (1991). Wait-Free Synchronization. *ACM Transactions on Programming Languages and Systems*, 13(1), 124–149.
- Herlihy, M., & Wing, J. M. (1990). Linearizability: A Correctness Condition for Concurrent Objects. *ACM Transactions on Programming Languages and Systems*, 12(3), 463–492.
- Michael, M. M., & Scott, M. L. (1996). Simple, Fast, and Practical Non-Blocking and Blocking Concurrent Queue Algorithms. In *Proceedings of the 15th ACM Symposium on Principles of Distributed Computing (PODC '96)*, pp. 267–275.
- Adve, S. V., & Gharachorloo, K. (1996). Shared Memory Consistency Models: A Tutorial. *IEEE Computer*, 29(12), 66–76.

## Licence

MIT — voir [LICENSE](./LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com/)
