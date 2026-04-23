# In-Memory KV

![CI](https://github.com/YanBatytskiy/in_memory_base/actions/workflows/ci.yml/badge.svg)
![Go](https://img.shields.io/badge/go-1.25-blue)
![License](https://img.shields.io/badge/license-MIT-green)
[![Go Report Card](https://goreportcard.com/badge/github.com/YanBatytskiy/in_memory_base)](https://goreportcard.com/report/github.com/YanBatytskiy/in_memory_base)

Educational in-memory key-value database in Go with a TCP API and write-ahead log recovery.

This repository is a learning project focused on the internals behind a small database service: request parsing, a TCP server/client pair, storage abstraction, an in-memory engine, WAL batching, recovery from segment files, and concurrency-safe data access.

## Features

- TCP server and interactive CLI client.
- Text command protocol with `SET`, `GET`, and `DEL`.
- Concurrent in-memory hash table protected by `sync.RWMutex`.
- Write-ahead log stored in rotating segment files.
- WAL batching by timeout, operation count, and byte volume.
- Startup recovery by replaying WAL records in LSN order.
- Unit tests for parsing, storage, WAL, initialization, and network behavior.

## Architecture

```text
cmd/server
  -> initialization
  -> network TCP server
  -> database command handler
  -> compute parser/validator
  -> storage facade
  -> in-memory engine
  -> WAL writer/recovery + hash table

cmd/cli
  -> network TCP client
```

The write path appends a record to the WAL first, waits for the WAL flush result, and then applies the operation to the in-memory hash table. On restart, the WAL reader replays segment files to rebuild state.

## Quick Start

Requirements:

- Go 1.25 or newer.

Run the server:

```bash
CONFIG_PATH=config/yaml/example.yaml go run ./cmd/server
```

In another terminal, run the CLI:

```bash
go run ./cmd/cli -- -address 127.0.0.1:3323
```

Try a few commands:

```text
SET name yan
GET name
DEL name
GET name
exit
```

## Example Session

End-to-end transcript showing SET / GET / DEL plus WAL recovery after a
server restart. Lines prefixed with `>` are typed into the CLI.

```text
# Terminal 1 — start the server
$ CONFIG_PATH=config/yaml/example.yaml go run ./cmd/server
[15:04:05.000] INFO: starting service {"logger level": "dev"}
[15:04:05.000] INFO: server listening {"address": "127.0.0.1:3323"}

# Terminal 2 — connect and run a few commands
$ go run ./cmd/cli -- -address 127.0.0.1:3323
[15:04:08.000] INFO: connected to server {"address": "127.0.0.1:3323"}

> SET user/1 alice
OK
> SET user/2 bob
OK
> GET user/1
VALUE alice
> DEL user/1
DELETED
> GET user/1
NOT_FOUND
> exit

# Terminal 1 — stop the server (Ctrl+C), then start it again.
# The WAL on disk is replayed before the listener accepts connections.
$ CONFIG_PATH=config/yaml/example.yaml go run ./cmd/server
[15:05:12.000] INFO: starting service {"logger level": "dev"}
[15:05:12.000] INFO: server listening {"address": "127.0.0.1:3323"}

# Terminal 2 — reconnect and verify persistence
$ go run ./cmd/cli -- -address 127.0.0.1:3323
> GET user/1
NOT_FOUND
> GET user/2
VALUE bob
```

## Docker

Multi-stage `Dockerfile` builds a static, distroless image (~8 MB) that
runs as `nonroot`. `docker-compose.yml` wires the server with a named
volume for the WAL and a container-friendly config (binds to `0.0.0.0`,
stores segments under `/storage/wal`).

```bash
docker-compose up --build
```

Connect with the CLI from the host:

```bash
go run ./cmd/cli -- -address 127.0.0.1:3323
```

WAL data persists across restarts via the `wal-data` volume. Remove it
with `docker-compose down -v` to start fresh.

## Command Protocol

Commands are line-oriented text sent over TCP.

| Command | Response |
| --- | --- |
| `SET key value` | `OK` |
| `GET key` | `VALUE value` or `NOT_FOUND` |
| `DEL key` | `DELETED` |

Command names are case-insensitive. Keys and values are currently single tokens and cannot contain spaces.

## Configuration

The server reads configuration from `CONFIG_PATH` when it is set, otherwise it uses environment/default values.

Example:

```yaml
engine_type: "in_memory"
network:
  engine_address: "127.0.0.1:3323"
  max_connections: 2
  max_message_size: 4096
  idle_timeout: 5m
  buffer_size: 4096
  type: tcp
logging:
  level: "dev"
wal:
  flushing_batch_timeout: 10ms
  flushing_batch_count: 2
  flushing_batch_volume: 10485760
  max_segment_size: 1073741824
  segment_storage_path: "./storage/wal"
  mask_name: "segment_"
```

## Testing

Run the main checks locally:

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
```

GitHub Actions runs the same checks on pushes and pull requests.

## Project Goals

This code is intentionally small enough to read end to end. It is meant to demonstrate:

- layered Go package design;
- interface-based testing with generated mocks;
- TCP connection handling and graceful shutdown;
- concurrency primitives and race-tested shared state;
- WAL persistence and recovery concepts;
- table-driven tests for edge cases.

## Limitations

This is an educational project, not a production database.

- Single-node only.
- No authentication or encryption.
- String-only keys and values.
- Values cannot contain spaces.
- No snapshots or compaction.
- WAL format is Go `gob`, intended for learning rather than long-term compatibility.

## License

MIT
