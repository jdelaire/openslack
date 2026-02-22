# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build ./...                          # Build all packages
./build.sh                              # Build both binaries to ./bin/
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

### Custom commands

Defined in `~/.openslack/commands.json` (outside the repo). Loaded at daemon startup as `ShellOp` instances. Each runs via `bash -l -c`. All default to `RiskLow`.

### Secrets

All secrets live in macOS Keychain (service: `openslack`), never in config files. Accounts: `telegram-bot-token`, `telegram-chat-id`, `totp-secret`.

## Conventions

- **Concurrency**: Registries use `sync.RWMutex`. Dispatcher limits concurrent ops with a buffered channel semaphore.
- **Testing**: Table-driven tests, dependency injection, mock implementations (e.g., `mockOp` in `registry_test.go`). Injectable clocks for time-dependent tests.
- **Logging**: `log/slog` with JSON handler to stdout.
- **Context timeouts**: 5s for socket connections, 30s for op execution, 10s for notification delivery.
