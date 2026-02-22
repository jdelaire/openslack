# OpenSlack Phase 1 MVP — Progress

## Batch 1: Project scaffold + Core types — DONE
- [x] `go mod init github.com/jdelaire/openslack`
- [x] `core/notification.go` — Notification struct (ID, Text, CreatedAt, Source)
- [x] `core/notifier.go` — Notifier interface (Name, Send)
- [x] `core/registry.go` — Notifier registry (Register, Default, Get)
- [x] `core/schema.go` — Request/Response JSON types + ValidateRequest()
- [x] Unit tests: 18 tests passing (schema validation + registry)

## Batch 2: Socket server — DONE
- [x] `core/server.go` — Unix domain socket listener, one-shot request/response
- [x] Stale socket cleanup, 0600 socket permissions, 0700 directory permissions
- [x] Schema validation dispatch, 8KB payload limit via LimitReader
- [x] Graceful shutdown with WaitGroup, socket file cleanup
- [x] Unit tests: 10 tests passing (notify, invalid JSON, permissions, stale cleanup, shutdown, multi-conn)

## Batch 3: Telegram notifier + Keychain — DONE
- [x] `adapters/telegram_notifier/notifier.go` — HTTP client calling sendMessage via PostForm
- [x] `internal/keychain/keychain.go` — Thin wrapper over `go-keyring` (service: openslack)
- [x] Unit tests: 5 tests passing (success, API error, network error, name, token in URL)

## Batch 4: Daemon + CLI wiring — DONE
- [x] `cmd/openslackd/main.go` — Wire keychain, notifier, registry, server; SIGINT/SIGTERM handling; slog JSON to stdout
- [x] `cmd/openslackctl/main.go` — Connect to socket, send notify request, print result; exit codes 0/1/2
- [x] `go build ./...` compiles cleanly
- [x] `go test ./...` — all 28 tests pass

## Verification Checklist — ALL PASSED
- [x] `go build ./...` compiles
- [x] `go test ./...` passes (28 tests)
- [x] Provision keychain secrets manually
- [x] Start daemon: `./openslackd`
- [x] Send: `./openslackctl notify "test message"`
- [x] Verify Telegram message received

## Phase 2: Inbound Telegram + Allowlisted Ops — DONE

### Batch 1: Core types + Policy + Op registry — DONE
- [x] `core/inbound.go` — InboundMessage struct + MessageHandler type
- [x] `core/receiver.go` — Receiver interface
- [x] `core/ops/registry.go` — Op interface + Registry (Register, Get, List)
- [x] `core/ops/registry_test.go` — 5 tests (register/get/list/duplicate/empty)
- [x] `core/policy/policy.go` — Policy with chat allowlist, 5-min freshness, update_id dedup (10k cap)
- [x] `core/policy/policy_test.go` — 6 tests (allowed/denied/stale/duplicate/pruning/empty)

### Batch 2: Telegram receiver adapter — DONE
- [x] `adapters/telegram_receiver/receiver.go` — Long-poll getUpdates, offset tracking, 30s poll timeout, 5s backoff
- [x] `adapters/telegram_receiver/receiver_test.go` — 6 tests (poll success, empty, skip no-text, offset, cancel, backoff)

### Batch 3: Dispatcher + initial ops — DONE
- [x] `core/dispatcher.go` — Authorize → parse → lookup → semaphore → execute → respond
- [x] `core/dispatcher_test.go` — 8 tests (authorized, unauthorized, unknown, stale, busy, error, non-command, parseCommand)
- [x] `core/ops/status.go` — /status returns uptime, Go version, goroutine count
- [x] `core/ops/help.go` — /help lists all ops sorted
- [x] `core/ops/ops_test.go` — 5 tests (status output/name, help output/sort/empty)

### Batch 4: Daemon wiring — DONE
- [x] `cmd/openslackd/main.go` — Parse chatID int64, wire ops/policy/dispatcher/receiver, background goroutine, WaitGroup shutdown
- [x] `go build ./...` compiles
- [x] `go test ./...` — all 58 tests pass

## File Layout
```
openslack/
├── go.mod
├── go.sum
├── cmd/
│   ├── openslackd/main.go      # daemon entry point
│   └── openslackctl/main.go    # CLI entry point
├── core/
│   ├── notification.go          # Notification struct
│   ├── notifier.go              # Notifier interface
│   ├── inbound.go               # InboundMessage + MessageHandler
│   ├── receiver.go              # Receiver interface
│   ├── dispatcher.go            # Command dispatcher
│   ├── dispatcher_test.go
│   ├── registry.go              # notifier registry
│   ├── registry_test.go
│   ├── schema.go                # Request/Response types + validation
│   ├── schema_test.go
│   ├── server.go                # Unix socket server
│   ├── server_test.go
│   ├── ops/
│   │   ├── registry.go          # Op interface + registry
│   │   ├── registry_test.go
│   │   ├── status.go            # /status op
│   │   ├── help.go              # /help op
│   │   └── ops_test.go
│   └── policy/
│       ├── policy.go            # Chat allowlist + freshness + dedup
│       └── policy_test.go
├── adapters/
│   ├── telegram_notifier/
│   │   ├── notifier.go          # Telegram Bot API adapter (outbound)
│   │   └── notifier_test.go
│   └── telegram_receiver/
│       ├── receiver.go          # Telegram long-poll receiver (inbound)
│       └── receiver_test.go
├── internal/
│   └── keychain/
│       └── keychain.go          # go-keyring wrapper
└── docs/
    ├── plan.md
    └── progress.md
```
