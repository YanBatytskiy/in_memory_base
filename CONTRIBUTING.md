# Contributing

Thanks for taking a look. This is a learning project, but contributions
that clarify the code, fix bugs or strengthen the test suite are welcome.

## Prerequisites

- **Go 1.25** or newer. The module declares `go 1.25` in `go.mod`.
- **[Task](https://taskfile.dev)** (`task --version`) — the Taskfile is the
  canonical place for build / test / lint commands. A Makefile is not
  provided.
- **[golangci-lint v2.11.x](https://golangci-lint.run)**. The Taskfile
  installs a pinned version into `./bin/` on first use:
  `task golangci-lint:install`.
- `gofumpt` and `gci` for formatting; again, `task formatters:install`
  pulls them into `./bin/`.

One-shot bootstrap:

```bash
task setup
```

## Workflow

1. Fork the repository and create a topic branch from `main`:
   `git checkout -b feature/<short-description>`.
2. Make your changes with clear, focused commits.
3. Run the full local pipeline before pushing:

   ```bash
   task check
   ```

   `check` runs `format → vet → lint → test → race`. All of those must
   pass. Integration tests and benchmarks are not in `check` — run them
   separately if your change touches them:

   ```bash
   task test:api     # //go:build integration scenarios
   ```

4. Push and open a Pull Request against `main`. Describe the problem the
   change solves and reference any related issue.

## Code style

- `gofumpt -extra` and `gci` formatting is enforced via the Taskfile and
  CI. Run `task format` before committing.
- Linter configuration lives in `.golangci.yml`. If you need to suppress
  a rule, use a `//nolint:<linter> // reason` comment with a short
  justification — blind `//nolint` without a reason will fail the
  `nolintlint` check.
- Exported APIs must have GoDoc comments starting with the identifier
  name (Effective Go style).

## Mocks

`mockery` generates testify-style mocks from interfaces listed in
`.mockery.yaml`. Regenerate after changing an interface:

```bash
task mocks
```

Commit regenerated mocks together with the interface change.

## Tests

- Unit tests live next to the code (`*_test.go`).
- Integration tests live under `internal/integration/` with the build tag
  `integration`.
- Race detector is expected to pass; avoid shared mutable state without
  a locking primitive from `internal/concurrency` or the standard
  `sync` package.

## Reporting issues

Open a GitHub issue with a minimal reproduction: Go version, exact
command, observed vs expected behaviour. For crashes, please include the
stack trace.
