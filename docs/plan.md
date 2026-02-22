# OpenSlack Engineering Plan

## Overview

OpenSlack is a sealed local worker daemon that bridges local processes to Telegram (and later other channels) without exposing the host to inbound network access. The system is built around strict security constraints: no arbitrary remote shell, strict allowlists, strict schemas, strict path controls, and defense in depth.

### Architecture

```
┌─────────────────────────────────────────────────────┐
│                     openslackd                      │
│                                                     │
│  ┌──────────┐  ┌─────────┐  ┌────────────────────┐ │
│  │ Receivers │→ │  Core   │→ │     Notifiers      │ │
│  │ (inbound) │  │ policy, │  │ (outbound)         │ │
│  │           │  │ routing,│  │                    │ │
│  └──────────┘  │ schema  │  └────────────────────┘ │
│                │ exec    │                          │
│  ┌──────────┐  │         │  ┌────────────────────┐ │
│  │  Tools   │← │         │→ │     Planner        │ │
│  │ (caps)   │  └─────────┘  │ (LLM, Phase 5)    │ │
│  └──────────┘               └────────────────────┘ │
│                                                     │
│  Unix Domain Socket (local IPC only)                │
└─────────────────────────────────────────────────────┘
```

**Core** owns policy, routing, schema validation, and execution orchestration. Everything else is a module implementing an interface.

---

## MVP — Phase 1: Sealed Local Notification Bus

**Goal:** Prove the local message bus and outbound notification pipeline with strong containment. Local processes send notifications to `openslackd`, which forwards them to Telegram. No inbound commands, no execution, no filesystem tools.

### What ships

| Component | Path | Description |
|---|---|---|
| Daemon | `cmd/openslackd/` | Long-running process, listens on Unix domain socket |
| CLI | `cmd/openslackctl/` | `openslackctl notify "text"` sends a notification |
| Core | `core/` | `Notification` struct, `Notifier` interface, notifier registry, socket server |
| Telegram adapter | `adapters/telegram_notifier/` | Implements `Notifier`, calls `sendMessage` |
| Docs | `docs/SECURITY.md`, `docs/API.md` | Security posture, local socket request/response schema |

### Key design decisions

- **IPC: Unix domain socket only.** No TCP. Socket file permissions (`0600`) enforce single-user access.
- **Secrets: macOS Keychain.** Bot token and chat ID are never in repo, shell history, plist, or scripts.
- **Lifecycle: launchd LaunchAgent.** Starts at login, restarts on crash, manages the socket lifecycle.
- **Schema validation on every request.** Payload size limit, text length limit, strict JSON schema. Invalid payloads are rejected without crashing the daemon.
- **Modular notifier interface.** Telegram is the default, but the `Notifier` interface and registry exist from day one so the channel can be swapped later.

### Implementation sequence

1. **Define core types and interfaces**
   - `core/notification.go` — `Notification` struct (text, timestamp, metadata)
   - `core/notifier.go` — `Notifier` interface (`Send(ctx, Notification) error`)
   - `core/registry.go` — Notifier registry with default selection

2. **Implement Telegram notifier adapter**
   - `adapters/telegram_notifier/notifier.go` — HTTP client calling Telegram Bot API `sendMessage`
   - Read bot token and chat ID from macOS Keychain at startup
   - Timeout and error handling on outbound HTTP calls

3. **Implement socket server**
   - `core/server.go` — Listen on Unix domain socket, accept connections, read JSON request, validate schema, dispatch to notifier
   - Enforce socket file permissions (`0600`, owned by current user)
   - Handle stale socket cleanup on startup

4. **Wire daemon entry point**
   - `cmd/openslackd/main.go` — Initialize Keychain retrieval, create Telegram notifier, register in core, start socket server
   - Graceful shutdown on SIGTERM/SIGINT
   - Structured logging (JSON, stdout)

5. **Build CLI**
   - `cmd/openslackctl/main.go` — Connect to socket, send `notify` JSON request, print result
   - Exit codes: 0 success, 1 validation error, 2 connection error

6. **launchd integration**
   - LaunchAgent plist template in `deploy/`
   - Documentation for `launchctl load/unload`

7. **Documentation**
   - `docs/SECURITY.md` — What is and isn't defended, secret management, socket permissions
   - `docs/API.md` — Socket request/response JSON schema

### Dependencies

- Go 1.22+
- Telegram bot token + chat ID (provisioned in Keychain)
- macOS Keychain (`security` CLI or `go-keychain` library)
- launchd (macOS LaunchAgent)

### Risks and mitigations

| Risk | Mitigation |
|---|---|
| Keychain access prompting/friction | Document `security` CLI provisioning; test headless access |
| Socket permission misconfiguration | Enforce `0600` in code; fail loudly if wrong |
| Stale socket file after crash | Remove and recreate on startup |
| Telegram rate limiting | Low risk at MVP volume; log and back off |

### Done when

- [ ] `launchd` starts `openslackd` at login and restarts on crash
- [ ] `openslackctl notify "hello"` delivers a Telegram message within seconds
- [ ] No bot token appears in repo, shell history, plist, or scripts
- [ ] Invalid payloads are rejected; daemon stays up
- [ ] Socket is not accessible to other users (`0600`)

---

## Phase 2: Inbound Telegram + Allowlisted Ops

**Goal:** Enable remote triggering of predefined safe operations from Telegram. No generic command execution.

### What ships

| Component | Path | Description |
|---|---|---|
| Receiver interface | `core/receiver.go` | `Receiver` interface, inbound message struct |
| Telegram receiver | `adapters/telegram_receiver/` | Long-polling `getUpdates`, offset tracking |
| Operation registry | `core/ops/` | Allowlisted operation definitions and executor |
| Inbound policy | `core/policy/` | Chat ID allowlist, message freshness window |

### Design

- **Receiver adapter** polls Telegram `getUpdates` with offset tracking. No webhooks, no inbound ports.
- **Authorization** checks: allowlisted `chat_id` only, message freshness window (reject stale messages), `update_id` dedup.
- **Operations are code, not config.** Each op is a Go function registered by name. No shell-out, no user-supplied arguments. Examples: `/status`, `/lock`, pattern-matched triggers.
- **Execution constraints:** Per-op timeout, max 1-2 concurrent ops.
- **Responses** flow back through the existing `Notifier` interface.

### Implementation sequence

1. Define `Receiver` interface and inbound message struct
2. Implement Telegram long-polling receiver with offset tracking
3. Build operation registry (register by name, lookup, execute)
4. Implement inbound policy checks (chat allowlist, freshness, dedup)
5. Wire receiver into daemon: poll → authorize → dispatch op → notify result
6. Implement initial ops: `/status`, `/help`
7. Add audit logging for all inbound attempts (accepted/rejected)

### Done when

- [ ] Only messages from the allowlisted chat ID are processed
- [ ] `/status` returns a deterministic response
- [ ] Unknown commands are rejected with help text
- [ ] Ops time out safely; daemon remains responsive
- [ ] No op can escape into arbitrary execution

---

## Phase 3: Security Hardening

**Goal:** Defend against Telegram account compromise, bot token leakage, brute force, and operator error.

### What ships

| Component | Path | Description |
|---|---|---|
| TOTP verifier | `core/auth/` | Time-based OTP verification, secret in Keychain |
| Policy engine | `core/policy/` | Risk tiers, approval requirements |
| Rate limiter | `core/ratelimit/` | Token bucket or sliding window |
| Threat model | `docs/THREAT_MODEL.md` | What is and isn't defended |

### Key additions

- **TOTP second factor** required for all remote ops (`/lock <totp>`)
- **Rate limiting + lockout** after failed TOTP attempts
- **Replay protection:** strict freshness window + `update_id` tracking
- **Two-step approval** for medium/high-risk actions: `/do X <totp>` returns nonce, `/approve <nonce> <totp>` executes
- **Least-privilege daemon user** (optional dedicated `openslack` macOS user)
- **Socket group ownership** (`0660`, group-limited)

### Done when

- [ ] Stolen bot token alone cannot execute ops (TOTP required)
- [ ] Excessive failed attempts trigger lockout
- [ ] High-risk ops require two-step approval
- [ ] Daemon runs with least privilege

---

## Phase 4: Capability-Limited Filesystem Tools

**Goal:** Safe file interactions within authorized whitelisted folders using capability-based tools. No general filesystem or shell access.

### What ships

| Component | Path | Description |
|---|---|---|
| Workspace registry | `core/workspaces/` | Root registry by ID, permissions |
| FS tools | `tools/fs/` | `fs.list`, `fs.read`, `fs.write` with strict schemas |
| Policy expansion | `core/policy/` | Per-root write rules, approval gates |

### Key constraints

- **Roots by ID, not path.** Config maps `repo_set`, `inbox`, etc. to real paths. Telegram messages never contain raw paths.
- **Path traversal defense:** canonicalization, clean joins, symlink escape checks.
- **Policy rules:** writes to `inbox` are auto-approved; writes elsewhere require two-step approval.
- **Caps on reads:** max bytes, max files per list. All access logged (metadata only).

### Done when

- [ ] Path escape attempts (`../`, symlinks) are rejected
- [ ] Reads are capped and logged
- [ ] Writes outside `inbox` require approval
- [ ] Tool surface remains small and auditable

---

## Phase 5: LLM Planner

**Goal:** Telegram messages are interpreted by an LLM to propose structured action plans. The LLM proposes; OpenSlack validates; tools enforce capabilities.

### What ships

| Component | Path | Description |
|---|---|---|
| Planner interface | `core/planner/` | `Planner` interface producing `Plan` (JSON) |
| LLM adapter | `planners/llm/` | Provider-agnostic LLM integration |
| Executor | `core/executor/` | Plan validator + step runner |
| Prompt injection docs | `docs/PROMPT_INJECTION.md` | Structural mitigations |

### Key constraints

- **Plans are data, not code.** The LLM outputs a JSON plan with tool calls. The executor validates every step against the tool allowlist, schema, and policy before running it.
- **Suggest mode by default.** LLM returns plan + summary; user approves via Telegram for any writes or risky actions.
- **Hard limits:** max steps per plan, max bytes, max files touched.
- **No dynamic plugin loading.** Tool set is fixed at compile time.

### Done when

- [ ] LLM cannot cause execution of non-allowlisted tools
- [ ] Plans exceeding limits are rejected
- [ ] Writes require explicit approval (or are confined to `inbox`)
- [ ] System remains deterministic under adversarial prompts

---

## Phase 6: Second Notification Channel

**Goal:** Prove the architecture supports multi-channel notifications without core rewrites.

### What ships

- New notifier adapter (e.g., Slack or Discord) in `adapters/<new_notifier>/`
- Configurable default notifier and per-message `via` field
- Regression tests confirming unchanged core behavior

### Done when

- [ ] Existing Telegram notifier works unchanged
- [ ] Switching default notifier requires only a config change
- [ ] No core API change required

---

## Phase 7: Operational Polish

**Goal:** Improve maintainability and safety without expanding capability surface.

### What ships

- Log rotation
- `/health` status endpoint
- Safe audit log export
- Packaging (Homebrew formula or installer script in `deploy/`)
- Automated Keychain provisioning helper
- Operational docs (setup, upgrade, recovery)
- Minimal test harness and CI

### Done when

- [ ] Upgrade path is documented and repeatable
- [ ] Failures are diagnosable from logs
- [ ] Security posture is unchanged

---

## Project structure (target)

```
openslack/
├── cmd/
│   ├── openslackd/          # daemon entry point
│   └── openslackctl/        # CLI entry point
├── core/
│   ├── notification.go      # Notification struct
│   ├── notifier.go          # Notifier interface
│   ├── registry.go          # notifier registry
│   ├── server.go            # Unix socket server
│   ├── receiver.go          # Receiver interface (Phase 2)
│   ├── ops/                 # allowlisted operations (Phase 2)
│   ├── auth/                # TOTP (Phase 3)
│   ├── policy/              # authorization, risk tiers (Phase 2-3)
│   ├── ratelimit/           # rate limiting (Phase 3)
│   ├── workspaces/          # root registry (Phase 4)
│   ├── planner/             # Planner interface (Phase 5)
│   └── executor/            # plan validator + runner (Phase 5)
├── adapters/
│   ├── telegram_notifier/   # outbound Telegram (Phase 1)
│   └── telegram_receiver/   # inbound Telegram (Phase 2)
├── planners/
│   └── llm/                 # LLM planner adapter (Phase 5)
├── tools/
│   └── fs/                  # filesystem tools (Phase 4)
├── deploy/
│   └── launchd/             # LaunchAgent plist (Phase 1)
├── docs/
│   ├── plan.md              # this document
│   ├── API.md               # socket API schema
│   ├── SECURITY.md          # security posture
│   ├── THREAT_MODEL.md      # threat model (Phase 3)
│   └── PROMPT_INJECTION.md  # LLM mitigations (Phase 5)
└── specs/
    └── openslack-dev-plan.md
```

## Phase dependency graph

```
Phase 1 (MVP: Notification Bus)
    │
    ▼
Phase 2 (Inbound Telegram + Ops)
    │
    ▼
Phase 3 (Security Hardening) ──────┐
    │                               │
    ▼                               ▼
Phase 4 (FS Tools)           Phase 6 (Second Channel)
    │
    ▼
Phase 5 (LLM Planner)
    │
    ▼
Phase 7 (Operational Polish)
```

Phase 6 can run in parallel with Phases 4-5 since it only depends on the notifier interface from Phase 1.
