# Architecture

This document describes how `in_memory_base` is put together internally:
how a request flows from the TCP socket to the hash table, how the write-
ahead log batches and flushes data, and how the service rebuilds its state
on startup.

For usage, configuration, and limitations see [README.md](README.md).

## Layer overview

```text
┌─────────────────────────────────────────────────────────────────┐
│  cmd/server              cmd/cli                                │
│     │                       │                                   │
│     ▼                       ▼                                   │
│  initialization          application_cli                        │
│     │ (wires components,     │ (REPL: parseFlags →              │
│     │  recovers WAL)         │  buildClient → replLoop)         │
│     ▼                       ▼                                   │
│  ┌───────────┐          ┌───────────┐                           │
│  │ network.  │◄────TCP──┤ network.  │                           │
│  │ TCPServer │          │ TCPClient │                           │
│  └─────┬─────┘          └───────────┘                           │
│        │ bytes → handler                                        │
│        ▼                                                        │
│  ┌─────────────┐    parse & validate     ┌─────────────┐        │
│  │  database.  │◄──────────────────────► │  compute.   │        │
│  │  Database   │                         │  Compute    │        │
│  └──────┬──────┘                         └─────────────┘        │
│         │ SET / GET / DEL                                       │
│         ▼                                                       │
│  ┌─────────────┐                                                │
│  │  storage.   │                                                │
│  │  Storage    │                                                │
│  └──────┬──────┘                                                │
│         │ CommandStorage + QueryStorage                         │
│         ▼                                                       │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                   in_memory.Engine                        │  │
│  │                                                           │  │
│  │   writes ──► IDGenerator ──► wal.Wal ──► HashTable        │  │
│  │                              (batch+fsync)                │  │
│  │                                                           │  │
│  │   reads  ────────────────────────────► HashTable          │  │
│  └───────────────┬───────────────────────────┬───────────────┘  │
│                  │                           │                  │
│                  ▼                           ▼                  │
│           ┌────────────┐              ┌────────────┐            │
│           │ filesystem │              │ hash_table │            │
│           │  (segments │              │  (map +    │            │
│           │   on disk) │              │  RWMutex)  │            │
│           └────────────┘              └────────────┘            │
└─────────────────────────────────────────────────────────────────┘
```

Each layer has a single responsibility and talks to the next one through
an interface, so the stack is easy to test and to swap parts of.

## Write path: `SET key value`

1. `TCPServer` reads up to `bufferSize` bytes from the connection and calls
   the user-supplied `TCPHandler`. The handler is `Database.DatabaseHandler`.
2. `DatabaseHandler` hands the raw string to `compute.ParseAndValidate`,
   which splits tokens and checks them against the small allowed charset.
3. `DatabaseHandler` dispatches to `handleSet`, which calls
   `Storage.Set(ctx, key, value)`.
4. `Storage.Set` forwards to `in_memory.Engine.Set`, which:
   - generates a monotonic LSN via `IDGenerator.Generate` (atomic counter),
   - stashes the LSN into `ctx` through `contextid.TxIDKey`,
   - calls the command side of the engine — `wal.Wal.Set(ctx, key, value)`.
5. `Wal.Set` constructs a `WriteRequest` wrapping a `Log{LSN, CommandID,
   Arguments}`, appends it to the in-memory batch under `sync.Mutex`, and
   blocks on the request's `Future` waiting for durability.
6. The background flusher goroutine (started by `Wal.Start`) fires when
   any of three thresholds is reached — elapsed time (`ticker`),
   operation count, or byte volume. It takes the batch, calls
   `walWriter.Write`:
   - encodes all records into one gob stream,
   - appends the buffer to the active segment file (`segment.Write` →
     `os.File.Write` + `Sync`; if the segment would exceed
     `maxSegmentSize`, it rotates to a fresh file),
   - resolves every `WriteRequest.promise` with the flush result.
7. After a successful flush the goroutine applies each record to the
   hash table (`HashTable.Set` / `Del`) in LSN order.
8. The caller unblocks, `Engine.Set` returns, the response string
   propagates back up — `Storage.Set` → `handleSet` returns `"OK"` —
   `TCPServer` writes it to the socket.

A `GET` short-circuits at step 4: `Engine.Get` reads the hash table
directly (no WAL interaction) and returns either the value or
`storage.ErrKeyNotFound`.

## Recovery path

On startup `initialization.CreateWal` invokes `Wal.Recovery` before the
TCP listener starts accepting connections:

1. `segment.GetList` lists every file in `segment_storage_path` sorted by
   name (file names embed nanosecond timestamps, so lexicographic sort
   gives chronological order).
2. For each segment file:
   - read the whole file into memory (`os.ReadFile`),
   - stream-decode records with `gob.Decoder.Decode` until `io.EOF`,
   - sort the records by LSN (one segment may have been flushed from
     reordered batches, so we normalise order within a segment),
   - apply every record to the hash table via the same `wal.apply`
     function that the flusher uses at runtime.
3. If any segment exists, open the most recent one in append mode and
   install it as the writer's active segment. If no segment exists,
   create a fresh one.

`Wal.Start` is launched next. From that moment on, the runtime write
path takes over.

## Concurrency model

| Primitive | Where | Why |
| --- | --- | --- |
| `sync.RWMutex` | `HashTable` | reads outnumber writes; `Get` takes RLock, `Set`/`Del` take the write lock |
| `sync.Mutex` | `Wal.batch` / `activeBatchVolume` | the in-flight batch is a critical section shared by producers and the flusher |
| `chan []WriteRequest` (buffered, size 5) | `Wal.batches` | hand-off of sealed batches from producers to the flusher |
| `concurrency.Semaphore` | `TCPServer` | bounds the number of connection-handling goroutines to `max_connections` |
| `context.Context` | everywhere | cancellation for graceful shutdown, deadlines for `net.Dialer.DialContext` / `net.ListenConfig.Listen` / `Engine.Get` |
| `concurrency.Promise[error]` / `Future[error]` | WAL | per-write one-shot durability signal, see below |

Every public method of `HashTable`, `Wal`, `TCPServer`, and `Storage` is
safe for concurrent use. Tests run with `-race` on every CI build.

## WAL batching

The WAL batches writes because a single `fsync` per operation would cap
throughput at the IOPS of the disk. Instead, records accumulate and
flush on whichever of these fires first:

- `flushing_batch_timeout` — upper bound on latency even when traffic
  is low (default `10ms`).
- `flushing_batch_count` — upper bound on the number of records per
  batch (default `100`).
- `flushing_batch_volume` — upper bound on encoded batch size in bytes
  (default `10 MiB`).

Records are encoded with `encoding/gob`. Gob is convenient for a learning
project — no IDL, reflection-based codec, handles evolving structs
reasonably — and deliberately not the right choice for a real database
(no forward compatibility across Go major versions, larger on-disk size
than a fixed-length binary format, and reflection overhead in hot paths).
This trade-off is called out in the README's Limitations section.

Segment rotation is triggered lazily: `filesystem.Segment.Write` checks
whether appending the current batch would cross `max_segment_size`; if
yes, it closes the current file and opens a new one before writing.

## Promise / Future

`internal/concurrency/promise.go` + `future.go` implement a tiny typed
one-shot channel primitive (using generics).

Why not a plain `chan error`? Three reasons:

1. **Directionality.** A bare channel can be written and read by anyone.
   `Promise` exposes only `Set`, `Future` exposes only `Get`. This
   enforces that the flusher resolves the result and the caller consumes
   it — never the other way.
2. **Idempotent `Set`.** If something tries to resolve the same promise
   twice, the second call is silently ignored instead of panicking on a
   closed channel. That matters for error paths where a goroutine might
   both bail out early and go through the normal notification code.
3. **Readability.** `record.FutureResponse().Get()` at the call site
   makes the intent explicit: "wait for this specific write to finish".

## Graceful shutdown

- `cmd/server/main.go` creates `ctx` from `signal.NotifyContext(SIGINT,
  SIGTERM)`.
- `TCPServer.HandleClientQueries` stops accepting new connections when
  `ctx.Done()` fires, calls `listener.Close()`, and waits on its
  `WaitGroup` for in-flight handlers to finish.
- The WAL flusher goroutine detects `ctx.Done()` inside its `select`,
  drains the `batches` channel, seals and flushes the in-memory batch,
  then returns.
- `main` waits up to 5 seconds for the above to complete, then exits.

## Trade-offs and limitations

Explicit limitations (repeated from README for quick reference):

- Single-node, single-binary. No replication, no leader election, no
  clustering.
- Keys and values are strings with no whitespace.
- Storage is an in-memory map; the working set must fit in RAM.
- WAL uses `encoding/gob` — the on-disk format is not guaranteed stable
  across Go toolchain changes and is not intended for long-term
  preservation.
- No snapshots, no compaction. WAL grows forever (rotating segments at
  `max_segment_size` but never deleting old ones).
- No authentication or transport encryption.

These limitations are deliberate — the project exists to make the
internals (parsing, request routing, WAL, recovery, concurrent hash
table, TCP server) readable end-to-end. Each omitted feature would
either blur the focus or require significantly more code than the
~2 500 lines the project targets.
