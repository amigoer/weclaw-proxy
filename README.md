# WeClaw-Proxy

[中文文档](README.zh.md)

A Go-based proxy adapter that bridges external AI agents into WeChat.

## Features

- 🔌 **Multi-Agent Support** — Built-in OpenAI-compatible adapter (GPT-4, vLLM, Ollama, etc.) and generic Webhook adapter
- 🔀 **Smart Routing** — Route messages to different agents by command prefix or user ID
- 💬 **Session Management** — Automatic conversation context and history tracking
- 📱 **QR Code Login** — One-scan WeChat connection
- ⚡ **Long Polling** — Real-time messaging via WeChat ilink protocol
- 🔄 **Reconnect Recovery** — Persistent sync cursor for seamless restarts
- 🖥️ **Admin Panel** — Web-based management UI built with React and shadcn/ui

## Quick Start

### 1. Build

```bash
make          # Build frontend + Go binary
# Or manually:
# cd web && npm install && npm run build
# go build -o weclaw-proxy ./cmd/weclaw-proxy/
```

### 2. Configure

```bash
cp configs/config.example.yaml configs/config.yaml
# Edit config.yaml with your Agent API keys
```

### 3. Login to WeChat

```bash
make login
# Or: ./weclaw-proxy --login --config configs/config.yaml
# Scan the QR code displayed in terminal with WeChat
```

### 4. Start the Service

```bash
export OPENAI_API_KEY=sk-xxx
make dev
# Or: ./weclaw-proxy --config configs/config.yaml
```

Once started, open `http://localhost:8080` to access the admin panel.

## Make Commands

| Command | Description |
|---------|-------------|
| `make` | Build frontend + Go binary |
| `make web` | Build frontend only |
| `make go` | Build Go binary only |
| `make dev` | Start in development mode |
| `make dev-web` | Start frontend dev server (hot reload) |
| `make login` | WeChat QR code login |
| `make lint` | Run code checks |
| `make clean` | Clean build artifacts |

## Admin Panel

The built-in web admin panel lets you manage agents and routing rules without editing YAML files.

- **Dashboard** — Connection status, agent count, active sessions
- **Agent Management** — Add, edit, delete agents online with instant effect
- **Routing Rules** — Visual configuration of prefix-based routing rules

The admin panel is built with React + shadcn/ui and embedded into the Go binary via `go:embed` — no separate deployment needed.

## Configuration

### Agent Adapters

Supported adapter types:

| Type | Description | Config Fields |
|------|-------------|---------------|
| `openai` | OpenAI ChatCompletion compatible | `api_key`, `base_url`, `model`, `system_prompt` |
| `webhook` | Generic Webhook forwarding | `base_url`, `api_key` |

Any agent with an HTTP endpoint can be integrated via the `webhook` type.

### Routing Rules

```yaml
routing:
  default_adapter: "openai-gpt4"
  rules:
    # Prefix match: "/claude hello" routes to the claude adapter
    - match:
        prefix: "/claude"
      adapter: "claude"
    # User ID match
    - match:
        user_ids: ["user@im.wechat"]
      adapter: "my-dify-bot"
```

### Built-in Commands

| Command | Description |
|---------|-------------|
| `/clear` | Clear conversation history |
| `/reset` | Reset conversation context |
| `/help` | Show help information |

## Project Structure

```
weclaw-proxy/
├── cmd/weclaw-proxy/main.go     # Entry point
├── internal/
│   ├── weixin/                   # WeChat ilink protocol
│   │   ├── types.go              # Protocol type definitions
│   │   ├── client.go             # API client
│   │   ├── auth.go               # QR code login
│   │   ├── poller.go             # Long-poll message listener
│   │   └── sender.go             # Message sender
│   ├── adapter/                  # Agent adapters
│   │   ├── adapter.go            # Interface definition
│   │   ├── openai.go             # OpenAI adapter
│   │   └── webhook.go            # Webhook adapter
│   ├── server/                   # Admin panel
│   │   ├── api.go                # REST API
│   │   ├── store.go              # Runtime config store
│   │   └── embed.go              # Frontend embedding
│   ├── router/router.go          # Message routing
│   ├── session/session.go        # Session management
│   └── config/config.go          # Configuration
├── web/                          # Admin panel frontend (React + shadcn/ui)
├── configs/config.example.yaml   # Example config
├── Makefile                      # Build commands
└── go.mod
```

## Protocol

Built on the WeChat ilink protocol (reverse-engineered from `@tencent-weixin/openclaw-weixin` v1.0.3). Core APIs:

- `POST /ilink/bot/getupdates` — Long-poll for incoming messages
- `POST /ilink/bot/sendmessage` — Send messages
- `POST /ilink/bot/getconfig` — Fetch bot configuration
- `POST /ilink/bot/sendtyping` — Typing status indicator
- `GET /ilink/bot/get_bot_qrcode` — Get login QR code

## License

MIT
