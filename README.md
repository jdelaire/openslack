# OpenSlack

OpenSlack is a **sealed local worker and notification bus** designed to securely bridge local processes with external messaging platforms (currently Telegram). 

It provides a safe, modular way for background jobs, scripts, and local services to send notifications without embedding API keys or secrets in their code. It also supports receiving inbound commands to execute a strictly allowlisted set of operations on the local machine.

## Architecture

OpenSlack is built on a simple, secure architectural model:
- **`openslackd` (Daemon)**: A background Go process that listens on a local Unix domain socket and polls for inbound commands. It safely retrieves credentials from the macOS Keychain and securely relays messages to the configured external adapter.
- **`openslackctl` (CLI)**: A command-line tool that connects to the local socket and sends strongly-typed JSON requests to the daemon.
- **Adapters**: Modular components that implement the `Notifier` (outbound) and `Receiver` (inbound) interfaces. Currently, only Telegram is supported, but the architecture allows for future integrations (e.g., Slack, Discord) without changing the core.

### Security First
- **No Inbound Ports**: The daemon listens exclusively on a local Unix socket (`0600` permissions), completely inaccessible from the network.
- **No Secrets in Configs**: API tokens, chat IDs, and TOTP secrets are stored and retrieved dynamically from the macOS Keychain.
- **Chat Allowlist**: Only inbound messages from specific pre-authorized Chat IDs are processed.
- **TOTP & Approvals**: Sensitive commands require a Time-based One-Time Password (TOTP). High-risk commands use a 2-step `/do` and `/approve` nonce-based flow.
- **Rate Limiting**: Failed authentications are securely rate-limited to prevent brute-force attacks.

## Current Status

OpenSlack has completed **Phase 3 (Security Hardening)** of its development plan. 
- ✅ Local socket server (`openslackd`)
- ✅ CLI interface (`openslackctl`)
- ✅ Telegram Outbound Notifier & Inbound Receiver
- ✅ Request Dispatcher & Op Registry
- ✅ Security Hardening (TOTP, 2-step approval, Policy limits, Rate Limiter)
- ✅ macOS Keychain Integration
- ✅ Config-driven custom commands (`~/.openslack/commands.json`)
- ✅ Connector system for external tool integrations (`~/.openslack/connectors.json`)
- 🚧 Future Phases: Apple Reminders connector, file system tools, LLM planning.

## Getting Started

### Prerequisites
- macOS (for Keychain integration)
- Go 1.22 or higher
- A Telegram Bot Token and your destination Chat ID

### Installation
Clone the repository and build all binaries:
```bash
git clone https://github.com/jdelaire/openslack.git
cd openslack
./build.sh
```

This produces `openslackd`, `openslackctl`, and `sample-connector` in `./bin/`.

### Configuration (Keychain)
Before running the daemon, you must store your Telegram credentials in the macOS Keychain under the service name `openslack`:
- **`telegram_bot_token`**: Your bot's HTTP API Token.
- **`telegram_chat_id`**: The target Chat ID to send messages to.
- **`totp_secret`**: (Optional) A Base32 TOTP secret for authenticating inbound commands.

*(A helper script or guide for provisioning these secrets may be added in the future).*

## Usage

1. **Start the Daemon:**
   Run `openslackd` in the background (or via `launchd` for persistence):
   ```bash
   ./openslackd
   ```

2. **Send a Notification (Outbound):**
   Use the CLI to dispatch a message:
   ```bash
   ./openslackctl notify "Hello from OpenSlack!"
   ```
   If successful, you will receive the message in your configured Telegram chat instantly.

3. **Remote Commands (Inbound):**
   Send commands to your Telegram bot (from your allowlisted Chat ID):
   - `/help` - List available commands and their risk levels.
   - `/status` - Check the daemon uptime and system status.
   - `/tomorrow <task description>` - Create a task that starts tomorrow and is reminded daily at 06:00 local time.
   - `/tasks` - List open tasks as `<id>: <description>`.
   - `/done <id>` - Mark a task as done.
   - `/sample.echo hello` - Call the sample connector's echo tool.
   - `/sample.time` - Get the current time from the sample connector.
   - Any custom commands defined in `~/.openslack/commands.json`.
   - Any connector tools defined in `~/.openslack/connectors.json`.

   For protected commands, you must append your TOTP code (e.g., `/sample.echo hello 123456`). High-risk commands will respond with a nonce, requiring you to confirm with `/approve <nonce> <totp>`.

## Tasks (MVP)

Task data is stored in a single JSON file:

- macOS path: `~/Library/Application Support/OpenSlack/tasks.json`

Schema:

- Top level: `next_id` and `tasks`
- Per task: `id`, `text`, `created_at` (RFC3339), `start_date` (YYYY-MM-DD local), `status` (`open` or `done`), `schedule` (`daily_6am`), `last_reminded_date` (YYYY-MM-DD or `null`)

Behavior:

- `/tomorrow <text>` creates a task with `start_date = tomorrow` and replies `<id>: <text>`.
- `/tasks` shows all open tasks sorted by ascending `id`; if none, replies `No open tasks.`.
- `/done <id>` replies with one of:
  - `Done: <id>`
  - `Unknown task: <id>`
  - `Already done: <id>`
- At local 06:00 every day, OpenSlack sends one aggregated reminder containing all open tasks where `start_date <= today` and `last_reminded_date != today`.
- If there are no tasks to remind, OpenSlack sends nothing.

Idempotency note:

- For at-most-once-per-day behavior across restarts, the daemon sets `last_reminded_date=today` and saves before sending.
- If sending fails after save, that day can be missed for those tasks (logged as an error). This is the chosen MVP tradeoff.

## Custom Commands

You can define shell-based commands via a JSON config file at `~/.openslack/commands.json`. This keeps personal scripts and paths out of the repository.

**Config format:**
```json
[
  {
    "name": "cfcnx-workouts",
    "description": "Show CFCNX weekly workouts",
    "command": "/path/to/your/script.sh",
    "workdir": "/path/to/working/directory"
  }
]
```

| Field | Required | Description |
|---|---|---|
| `name` | Yes | The command name (used as `/name` in Telegram) |
| `description` | Yes | Shown in `/help` output |
| `command` | Yes | Shell command or script path to execute |
| `workdir` | No | Working directory for the command |

Commands are executed via `bash -l -c` for a full login shell environment. All custom commands default to `RiskLow` (require TOTP).

If the config file is missing, the daemon starts normally with no custom commands. If the file exists but contains invalid JSON or entries with missing required fields, the daemon exits with an error.

## Connectors

Connectors extend OpenSlack with tools implemented as **separate executables**. They communicate with the daemon over a strict JSON protocol via stdin/stdout — no dynamic code loading, no shell evaluation.

### How it works

1. The daemon reads `~/.openslack/connectors.json` at startup.
2. Each configured connector is spawned as a child process.
3. Connector tools are registered as Telegram commands using `connector.tool` naming (e.g., `/sample.echo`).
4. Only tools explicitly listed in the config allowlist can be called.

### Invoking from Telegram

```
/sample.echo hello world          # Calls sample connector's echo tool
/sample.time                      # Returns current timestamp
/help                             # Lists all commands including connector tools
```

If TOTP is enabled, append your code:
```
/sample.echo hello world 123456
```

The args string after the command name is passed to the connector as `{"text": "..."}`. If you pass raw JSON (starting with `{`), it's forwarded directly.

### Configuration

Create `~/.openslack/connectors.json`:

```json
{
  "connectors": {
    "sample": {
      "exec": "/path/to/bin/sample-connector",
      "tools": ["echo", "time"]
    }
  },
  "limits": {
    "req_max_bytes": 4096,
    "resp_max_bytes": 16384,
    "call_timeout_ms": 10000
  }
}
```

| Field | Required | Description |
|---|---|---|
| `connectors.<name>.exec` | Yes | Absolute path to the connector binary |
| `connectors.<name>.tools` | Yes | Allowlisted tool names this connector may serve |
| `limits.req_max_bytes` | No | Max request payload size (default: 4096) |
| `limits.resp_max_bytes` | No | Max response payload size (default: 16384) |
| `limits.call_timeout_ms` | No | Per-call timeout in milliseconds (default: 10000) |

If the config file is missing, the daemon starts normally with no connectors. Connector names must not contain dots.

### Creating a new connector

A connector is any executable that:

1. Reads newline-delimited JSON requests from **stdin**.
2. Writes newline-delimited JSON responses to **stdout**.
3. Logs to **stderr** (never protocol data).

**Request format:**
```json
{"version":"v1","id":"req_001","tool":"echo","args":{"text":"hello"}}
```

**Success response:**
```json
{"version":"v1","id":"req_001","ok":true,"data":{"text":"hello"}}
```

**Error response:**
```json
{"version":"v1","id":"req_001","ok":false,"error":{"code":"INVALID_ARGS","message":"text is required"}}
```

Every connector must also handle `tool: "__introspect"` and return its name, version, and tool list.

See `connectors/sample/main.go` for a complete working example. To add a new connector:

1. Create your binary (any language) under `connectors/<name>/`.
2. Add it to `build.sh`.
3. Add its entry to `~/.openslack/connectors.json`.
4. Restart the daemon.

No changes to core code are required.

### Security guardrails

- Connectors are spawned via `exec.Command` with args array — no shell.
- Only connectors and tools listed in config can be called.
- Payload size limits are enforced on both request and response.
- Per-call timeouts are enforced; a slow connector does not block the daemon.
- Connector crash returns an error to the caller; the daemon stays up.

## Development

The codebase is structured to be modular and testable:
- `core/`: Interface definitions, socket server, routing, ops registry, policy, authentication (TOTP), and schema validation.
- `core/connector/`: Connector protocol, config, process manager, tool router, and ops bridge.
- `adapters/`: External integration implementations (e.g., `telegram_notifier`, `telegram_receiver`).
- `connectors/`: Connector binaries (e.g., `sample/`).
- `internal/`: Internal utilities (e.g., `keychain`).
- `cmd/`: Application entry points (`openslackd`, `openslackctl`).

Run tests using:
```bash
go test ./...
```
