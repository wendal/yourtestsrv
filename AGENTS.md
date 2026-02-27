# AGENTS.md

This repository is a Go 1.21 network test server (TCP/UDP/HTTP/MQTT).
Use this guide for build, lint, tests, and local conventions.

## Quick Facts
- Module: `yourtestsrv`
- Go version: `1.21` (see `go.mod`)
- CI: GitHub Actions runs `go test ./...` (see `.github/workflows/go.yml`)

## Build / Run
- Build binary: `go build -o yourtestsrv cmd/server/main.go`
- Run help: `./yourtestsrv --help`
- Run all servers: `./yourtestsrv serve-all --config config.json`
- Run all TLS servers: `./yourtestsrv serve-all-tls --config config.json`

## Tests
- Run all tests: `go test ./...`
- Run tests for one package: `go test ./internal/http`
- Run a single test by name: `go test ./internal/http -run ^TestHTTPBasic$`
- Run a single test with verbose output: `go test -v ./internal/http -run ^TestHTTPBasic$`
- Repeat a test (flakiness check): `go test ./internal/udp -run ^TestUDPEcho$ -count=10`

## Lint / Format
- Format all Go code: `gofmt -w cmd internal`
- Check formatting only: `gofmt -l cmd internal`
- Vet: `go vet ./...`

Note: there is no configured linter (golangci-lint, staticcheck, etc.).
If you add one, document it here and in CI.

## Project Layout
- `cmd/server/main.go`: CLI entry point and server startup.
- `internal/config`: config types + JSON parsing.
- `internal/tcp`, `internal/udp`, `internal/http`, `internal/mqtt`: protocol servers + tests.
- `config.json`: default config example used by CLI.

## Code Style Guidelines

### Imports
- Use standard `gofmt` import grouping (stdlib, blank line, local).
- Keep import aliases only when needed for conflicts or clarity
  (e.g., `httpServer "yourtestsrv/internal/http"`).

### Formatting
- Run `gofmt` on all Go files you touch.
- Prefer tabs and standard Go formatting; do not hand-format.

### Naming
- Go naming conventions: `CamelCase` for exported identifiers,
  `camelCase` for unexported.
- Avoid abbreviations unless standard (e.g., `ctx`, `cfg`, `conn`).
- Use action-oriented function names (`ListenAndServe`, `handleConn`).

### Types and Structs
- Keep structs focused and small; group related fields together.
- Use `time.Duration` for time values; expose JSON as string durations
  via wrapper types (see `config.Duration`).
- Prefer `Handler` interfaces plus `HandlerFunc` adapters, consistent with
  existing server packages.

### Error Handling
- Return errors to caller where possible; log only at top-level boundaries
  (e.g., server loop, CLI entry).
- Preserve context in errors with `fmt.Errorf("...: %w", err)` when bubbling.
- In server loops, treat `context.Canceled` as a normal shutdown signal.

### Concurrency / Cancellation
- Respect `context.Context` for shutdown and read deadlines.
- On Accept/read loops, check `ctx.Done()` and exit cleanly.
- For goroutines started per connection/packet, ensure cleanup via
  `defer wg.Done()` and `defer conn.Close()`.

### Networking Behavior
- Default listeners bind to `0.0.0.0` and use configured ports.
- TLS listeners require `cert.pem` and `key.pem`; tests generate temp certs.
- When emulating special scenarios, keep behavior deterministic in tests.

### Testing Conventions
- Tests live alongside packages in `*_test.go`.
- Use `t.Helper()` in test helpers.
- Use ephemeral ports (`127.0.0.1:0`) to avoid conflicts.
- For network readiness, poll with a short deadline (see `waitTCP/HTTP/MQTT`).

### Logging
- Use `log.Printf` for server events; keep messages short and actionable.
- Avoid logging in tight loops unless necessary for tests.

### JSON / Config
- Config is JSON with snake_case keys; keep tags in `internal/config`.
- Do not add external deps for config parsing unless required.

## Cursor / Copilot Rules
- No `.cursor/rules/`, `.cursorrules`, or `.github/copilot-instructions.md`
  were found in this repository at the time of writing.

## When Adding New Features
- Keep new protocols or scenarios inside `internal/<protocol>`.
- Add tests that cover basic behavior plus one error/edge case.
- Update README usage examples if CLI flags change.

## Common Commands Summary
- Build: `go build -o yourtestsrv cmd/server/main.go`
- Test all: `go test ./...`
- Test single: `go test ./internal/tcp -run ^TestTCPEcho$`
- Format: `gofmt -w cmd internal`
- Vet: `go vet ./...`
