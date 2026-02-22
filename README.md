# OpenSlack

OpenSlack is a **sealed local worker and notification bus** designed to securely bridge local processes with external messaging platforms (currently Telegram). 

It provides a safe, modular way for background jobs, scripts, and local services to send notifications without embedding API keys or secrets in their code.

## Architecture

OpenSlack is built on a simple, secure architectural model:
- **`openslackd` (Daemon)**: A background Go process that listens on a local Unix domain socket. It safely retrieves credentials from the macOS Keychain and securely relays messages to the configured external adapter.
- **`openslackctl` (CLI)**: A command-line tool that connects to the local socket and sends strongly-typed JSON requests to the daemon.
- **Adapters**: Modular components that implement the `Notifier` interface. Currently, only Telegram is supported, but the architecture allows for future integrations (e.g., Slack, Discord) without changing the core.

### Security First
- **No Inbound Ports**: The daemon listens exclusively on a local Unix socket (`0600` permissions), completely inaccessible from the network.
- **No Secrets in Configs**: API tokens and Chat IDs are stored and retrieved dynamically from the macOS Keychain.
- **Strict Validation**: All local IPC payloads are schema-validated with strict size and length constraints.

## Current Status

OpenSlack is currently in **Phase 1 (MVP)** of its development plan. 
- ✅ Local socket server (`openslackd`)
- ✅ CLI interface (`openslackctl`)
- ✅ Telegram Outbound Notifier
- ✅ macOS Keychain Integration
- 🚧 Future Phases: Remote triggering, allowlisted operations, capable file system tools, and LLM planning.

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

*(A helper script or guide for provisioning these secrets may be added in the future).*

## Usage

1. **Start the Daemon:**
   Run `openslackd` in the background (or via `launchd` for persistence):
   ```bash
   ./openslackd
   ```

2. **Send a Notification:**
   Use the CLI to dispatch a message:
   ```bash
   ./openslackctl notify "Hello from OpenSlack!"
   ```
   If successful, you will receive the message in your configured Telegram chat instantly.

## Development

The codebase is structured to be modular and testable:
- `core/`: Interface definitions, socket server, routing, and schema validation.
- `adapters/`: External integration implementations (e.g., `telegram_notifier`).
- `internal/`: Internal utilities (e.g., `keychain`).
- `cmd/`: Application entry points (`openslackd`, `openslackctl`).

Run tests using:
```bash
go test ./...
```
