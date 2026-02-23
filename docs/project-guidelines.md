# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build ./...                          # Build all packages
./build.sh                              # Build all binaries to ./bin/
go test ./...                           # Run all tests
go test ./core/ops/...                  # Run tests for a single package
go test -run TestShellOpExecute ./core/ops/...  # Run a single test
```

No linter or formatter is configured beyond standard `go vet` (run automatically by `go test`).

## Architecture

OpenSlack is a local daemon that bridges shell commands and notifications with Telegram. Two binaries:

- **`openslackd`** — Daemon that listens on a Unix socket (`~/.openslack/openslack.sock`) for outbound notifications and polls Telegram for inbound commands.
- **`openslackctl`** — CLI that sends JSON requests to the daemon socket.

### Inbound command flow (Telegram → local execution)

```
Telegram getUpdates (long-poll)
  → telegram_receiver.poll()
  → Dispatcher.Handle(InboundMessage)
  → Policy.Authorize (chat allowlist + freshness + dedup)
  → RateLimiter.Check
  → parseCommand → ops.Registry.Get
  → Risk-level gating (None/Low/High)
  → Op.Execute (30s timeout, max 2 concurrent via semaphore)
  → Notifier.Send (response back to Telegram)
```

### Outbound notification flow (local → Telegram)

```
openslackctl notify "text"
  → Unix socket JSON request
  → Server.handleConnection (5s deadline, 8KB limit)
  → Notifier.Send → Telegram Bot API sendMessage
```

### Key interfaces

**`ops.Op`** — All commands implement this. Register in `ops.Registry`. Default risk is `RiskLow` (TOTP required). Implement `RiskClassifier` to override.

**`core.Notifier`** / **`core.Receiver`** — Adapter interfaces for messaging platforms. Currently only Telegram.

**Security interfaces** (`TOTPVerifier`, `RateLimiter`, `ApprovalStore`) — Injected into Dispatcher via `WithSecurity()`. If TOTP secret isn't in keychain, security is disabled gracefully.

### Connector system

Connectors are separate executables that extend OpenSlack with new tools. They communicate via JSON over stdin/stdout using a versioned protocol (`v1`).

- **`core/connector/`** — Protocol types, config loader, process manager, and tool router.
- **`connectors/sample/`** — Example connector binary with `echo`, `time`, and `sleep` tools.

Tool calls use `connector.tool` format (e.g., `sample.echo`). The router splits the name, validates the connector and tool against the allowlist in config, and dispatches via the manager.

Config lives at `~/.openslack/connectors.json`:
```json
{
  "connectors": {
    "sample": { "exec": "./bin/sample-connector", "tools": ["echo", "time"] }
  },
  "limits": { "req_max_bytes": 4096, "resp_max_bytes": 16384, "call_timeout_ms": 10000 }
}
```

Security: no dynamic loading, no shell execution, strict allowlist, payload size limits, per-call timeouts.

### Custom commands

Defined in `~/.openslack/commands.json` (outside the repo). Loaded at daemon startup as `ShellOp` instances. Each runs via `bash -l -c`. All default to `RiskLow`.

### Secrets

All secrets live in macOS Keychain (service: `openslack`), never in config files. Accounts: `telegram-bot-token`, `telegram-chat-id`, `totp-secret`.

## Conventions

- **Concurrency**: Registries use `sync.RWMutex`. Dispatcher limits concurrent ops with a buffered channel semaphore.
- **Testing**: Table-driven tests, dependency injection, mock implementations (e.g., `mockOp` in `registry_test.go`). Injectable clocks for time-dependent tests.
- **Logging**: `log/slog` with JSON handler to stdout.
- **Context timeouts**: 5s for socket connections, 30s for op execution, 10s for notification delivery.
