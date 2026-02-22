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
- 🚧 Future Phases: Capable file system tools, and LLM planning.

## Getting Started

### Prerequisites
- macOS (for Keychain integration)
- Go 1.22 or higher
- A Telegram Bot Token and your destination Chat ID

### Installation
Clone the repository and build the binaries:
```bash
git clone https://github.com/jdelaire/openslack.git
cd openslack
go build ./...
```

The executables `openslackd` and `openslackctl` will be generated in the root directory.

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
   - Any custom commands defined in `~/.openslack/commands.json` (see below).

   For protected commands, you must append your TOTP code (e.g., `/command 123456`). High-risk commands will respond with a nonce, requiring you to confirm with `/approve <nonce> <totp>`.

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

## Development

The codebase is structured to be modular and testable:
- `core/`: Interface definitions, socket server, routing, ops registry, policy, authentication (TOTP), and schema validation.
- `adapters/`: External integration implementations (e.g., `telegram_notifier`, `telegram_receiver`).
- `internal/`: Internal utilities (e.g., `keychain`).
- `cmd/`: Application entry points (`openslackd`, `openslackctl`).

Run tests using:
```bash
go test ./...
```
