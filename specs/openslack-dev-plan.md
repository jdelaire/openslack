# OpenSlack — Structured Development Plan (Modular, Security-Sealed)

## Problem statement

Build a **sealed local worker** (“OpenSlack”) that can:

1. **Receive** messages from Telegram (outbound long-polling; no inbound ports).
2. **Execute** a strictly allowlisted set of local operations and/or file interactions **within authorized roots**.
3. **Send** notifications from local processes to Telegram through the same worker.
4. Remain **modular** so outbound notification channels can change later (Telegram now, others later) without rewriting the core.

Security constraints are first-class:
- No arbitrary remote shell.
- Strict allowlists, strict schemas, strict path controls.
- Defense in depth (chat allowlist + strong auth + policy gates + capability-based tools).

---

## Architectural north star (stable across phases)

A small **core** with narrow interfaces, plus **adapters** and **tools**:

- **Core**: policy, routing, schema validation, execution orchestration
- **Receivers** (inbound): Telegram long-polling now, others later
- **Notifiers** (outbound): Telegram now, others later
- **Tools**: capability-limited actions (fs read/write in root IDs, git, etc.)
- **Planner** (later): LLM proposes a plan; core validates and executes via tools

> This preserves architectural stability: new channels and features are added as modules implementing interfaces, not by modifying core behavior.

---

## Phase 1 — MVP: Sealed Local Notification Bus (Outbound only)

### Objective
Prove the **local message bus** and **notification pipeline** with strong containment:
- Local processes can send notifications to OpenSlack.
- OpenSlack forwards notifications to Telegram reliably.
- No secrets are embedded in scripts or configs.
- Modular boundary exists so Telegram can later be swapped for another notifier.

### Hypothesis validated by MVP
A **single always-on local daemon** with a **Unix domain socket** interface can provide a secure, reliable, low-friction notification bridge to Telegram **without exposing the machine to inbound network access**.

### Scope
**Included**
- Go daemon: `openslackd`
- Go CLI: `openslackctl notify "text"`
- Local IPC via **Unix domain socket** (no TCP)
- Local API: `notify` request (JSON) with strict schema
- Telegram **outbound** notifier adapter (`sendMessage`)
- Secret retrieval via **macOS Keychain** (token + chat ID)
- `launchd` integration to keep daemon alive
- Input constraints:
  - payload size limit
  - text length limit
  - schema validation
- Minimal structured logging (success/failure, timestamp)

**Excluded**
- Telegram inbound (no `getUpdates` yet)
- Any local command execution
- Any filesystem tooling
- TOTP / multi-step approvals / rate limiting
- Multiple notification channels
- LLM integration

### Key deliverables
- `core/` with:
  - `Notification` struct
  - `Notifier` interface
  - Registry with default notifier
  - Local socket server handling `notify`
- `adapters/telegram_notifier/` implementing `Notifier`
- `cmd/openslackd/` wiring + launchd guidance
- `cmd/openslackctl/` CLI
- `docs/SECURITY.md` (MVP security posture and non-goals)
- `docs/API.md` (local socket request/response schema)

### Dependencies
- Go 1.22+
- Telegram bot token + chat ID
- macOS Keychain access
- launchd (LaunchAgent)

### Risks
- Keychain access friction (permissions, prompting)
- socket file permission misconfiguration
- Telegram API rate limiting if spammed (low risk in MVP)
- operational reliability (daemon restarts, stale socket)

### Validation criteria
- `launchd` starts `openslackd` at login and restarts on crash.
- `openslackctl notify "hello"` results in a Telegram message within seconds.
- No bot token appears in repo, shell history, plist, or scripts.
- Invalid payloads are rejected and do not crash the daemon.
- Socket is not accessible to other users on the machine (permissions enforced).

---

## Phase 2 — Inbound Telegram + Allowlisted Ops (No LLM)

### Objective
Enable **remote triggering** of a small set of **predefined safe operations** from Telegram while preserving the Phase 1 bus and notifier interface.

### Scope
**Included**
- Telegram **Receiver** adapter (long polling via `getUpdates`)
- Strict inbound authorization:
  - allowlisted `chat_id`
  - message freshness window
  - offset tracking (no duplicates)
- Allowlisted operations are strictly pre-defined by implementation:
  - Internal functions or fixed scripts with no args (e.g., `/lock`, `/status`, `/update_repo`).
  - Pattern matching for specific messages (e.g., if we poll the message "rednote-bot like crossfit", a dedicated implementation matches this text and triggers the appropriate behavior on the computer).
- Execution constraints:
  - per-op timeout
  - max concurrent ops (very small)
- Response notifications back to Telegram (reuse Notifier)

**Excluded**
- Any generic “run command” facility
- Arbitrary arguments from Telegram
- LLM planner
- Filesystem read/write tools (unless trivially needed for a safe op)
- Multi-user / group chats

### Key deliverables
- `core/receiver.go` interface + message struct
- `adapters/telegram_receiver/` long polling implementation
- `core/ops/` allowlisted operation registry
- `core/policy/` minimal inbound checks (chat allowlist, freshness)
- Telemetry: basic audit log of attempted ops (accepted/rejected)

### Dependencies
- Phase 1 stable
- Telegram bot configured (private usage)
- Stable system clock (freshness checks)

### Risks
- Accidental widening of scope into remote shell
- Duplicate processing if offset handling is buggy
- Safety risks from “safe” ops that actually do risky things

### Validation criteria
- Only messages from the allowlisted chat ID are processed.
- `/status` returns a deterministic response.
- Unknown commands are rejected with help text.
- Ops time out safely; daemon remains responsive.
- No op can escape into arbitrary execution or read sensitive files.

---

## Phase 3 — Security Hardening: Strong Auth + Policy Gates

### Objective
Make OpenSlack resilient to the realistic failures:
- Telegram account compromise
- bot token leakage
- brute force attempts
- operator mistakes on destructive actions

### Scope
**Included**
- TOTP (second factor) required for remote ops:
  - `/lock <totp>`
- Rate limiting + lockout after failed attempts
- Replay protection:
  - strict freshness window + update_id tracking
- Two-step approval flow for any medium-risk action:
  - `/do X <totp>` → returns nonce
  - `/approve <nonce> <totp>` executes
- Dedicated low-privilege macOS user option:
  - run daemon under `openslack` user
- Socket access tightened via group ownership:
  - `0660` socket, group-limited

**Excluded**
- LLM integration
- Multi-device federation / key rotation automation
- Web UI

### Key deliverables
- `core/auth/` TOTP verifier (Keychain stored secret)
- `core/policy/` risk tiers and approval requirements
- `core/ratelimit/` minimal token bucket or sliding window
- `docs/THREAT_MODEL.md` (what is and isn’t defended)

### Dependencies
- Authenticator app for TOTP
- Keychain secret provisioning scripts/docs
- Optional: dedicated user setup steps

### Risks
- Usability friction causing the user to bypass security
- Time drift causing TOTP failures

### Validation criteria
- Stolen bot token alone cannot execute ops (TOTP required).
- Excessive failed attempts trigger lockout.
- High-risk ops require approval and cannot auto-run.
- Daemon runs with least privilege; compromise blast radius reduced.

---

## Phase 4 — Capability-Limited Filesystem Tools (Whitelisted Roots)

### Objective
Enable safe file interactions within **authorized whitelisted folders** using **capability-based tools**, without granting the system a general filesystem or shell capability.

### Scope
**Included**
- Workspace root registry by **ID** (not raw paths):
  - `repo_set`, `inbox`, etc.
- Tools with strict schemas and hard constraints:
  - `fs.list(root_id, glob, max_files)`
  - `fs.read(root_id, path, max_bytes)`
  - `fs.write(root_id, path, content, mode=append|overwrite)` with policy
- Path traversal and symlink escape protections:
  - canonicalization, clean joins, escape checks
- Policy rules:
  - writes allowed without approval only in `inbox`
  - writes elsewhere require approval

**Excluded**
- Arbitrary command execution in folders
- Auto-committing to git
- File upload/download over Telegram beyond small text snippets

### Key deliverables
- `core/workspaces/` root registry + permissions
- `tools/fs/` capability-limited filesystem operations
- Expanded policy table for tool calls
- Audit logs for file reads/writes (metadata only)

### Dependencies
- Phase 3 security posture (auth + approvals)
- Clear definition of whitelisted roots and permissions

### Risks
- Symlink/path canonicalization edge cases
- Excessive data exfiltration via read tools
- Tool schema creep

### Validation criteria
- Attempts to escape roots are rejected (../, symlinks).
- Reads are capped and logged.
- Writes outside inbox require approval.
- Tool surface remains small and auditable.

---

## Phase 5 — LLM Planner (Plan → Validate → Execute)

### Objective
Allow Telegram messages to be **interpreted by an LLM** to propose actions, while maintaining the security invariant:
> LLM proposes; OpenSlack validates; tools enforce capabilities.

### Scope
**Included**
- `Planner` interface producing a structured `Plan` (JSON)
- Strict plan validation:
  - tool allowlist
  - schema validation per tool
  - policy checks (approval gates)
  - limits (max steps, max bytes, max files)
- Execution engine runs validated steps via tools
- “Suggest mode” default:
  - LLM returns plan + summary
  - user approves via Telegram for any writes or risky actions

**Excluded**
- Fully autonomous destructive actions
- Arbitrary code execution
- Dynamic plugin loading

### Key deliverables
- `core/planner/` interface
- `planners/llm/` minimal integration (provider-agnostic adapter)
- `core/executor/` plan validator + runner
- `docs/PROMPT_INJECTION.md` (structural mitigations + examples)

### Dependencies
- A chosen LLM provider or local LLM runtime
- Strong Phase 3–4 policy enforcement in place

### Risks
- Prompt injection via Telegram text or repo content
- Over-permissive tool set turning into remote control
- Cost/latency issues

### Validation criteria
- LLM cannot cause execution of non-allowlisted tools.
- Plans that exceed limits are rejected.
- Writes require explicit approval (or are confined to inbox).
- System remains deterministic under adversarial prompts.

---

## Phase 6 — Second Notification Channel (Prove Modularity)

### Objective
Demonstrate that the architecture truly supports multi-channel notifications without core rewrites.

### Scope
**Included**
- Add a second notifier adapter (e.g., Slack or Discord)
- Configurable default notifier and per-message `via`
- Regression tests confirming unchanged core behavior

**Excluded**
- Full multi-channel routing rules engine
- Advanced templates or formatting

### Key deliverables
- `adapters/<new_notifier>/`
- Updated config schema + docs
- Integration tests for notifier registry behavior

### Dependencies
- Credentials for the new channel

### Risks
- Adapter-specific rate limits and formatting quirks

### Validation criteria
- Existing Telegram notifier continues to work unchanged.
- Switching default notifier requires only config change.
- No core API change required.

---

## Phase 7 — Operational Polish (Only after utility proven)

### Objective
Improve maintainability and safety without expanding capability surface.

### Scope
**Included**
- Log rotation
- Better status reporting (`/health`)
- Safe export of audit logs
- Packaging (Homebrew formula or simple installer)
- Automated key provisioning helper

**Excluded**
- Web dashboard
- Remote management portal
- Overbuilt observability stack

### Key deliverables
- Installer script / packaging
- Operational docs (setup, upgrade, recovery)
- Minimal test harness and CI

### Dependencies
- Real-world usage feedback

### Risks
- Turning “polish” into scope creep

### Validation criteria
- Upgrade path is documented and repeatable.
- Failures are diagnosable from logs.
- Security posture remains unchanged.

---

## Traceability map (problem → phases)

- Local notifications to Telegram → **Phase 1**
- Telegram remote trigger → **Phase 2**
- “Only me” + sealed security → **Phase 3**
- Whitelisted folder actions → **Phase 4**
- LLM-mediated actions → **Phase 5**
- Swap/extend channels → **Phase 6**
- Operate reliably long-term → **Phase 7**
