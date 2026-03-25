# WeClaw-Proxy

> WeChat Open-Platform AI Agent Proxy — Connect any AI Agent to WeChat in one step

[中文文档](README_zh.md)

## ✨ Features

- 🔌 **Multi-Agent Support** — OpenAI, DeepSeek, Ollama, Dify, Coze, Webhook, and **Local CLI** (Codex / Claude Code / Gemini CLI)
- 📱 **Multi-Account WeChat** — Bind and manage multiple WeChat accounts simultaneously
- 🧠 **Smart Routing** — LLM-powered automatic message classification, plus manual `/command` prefix routing
- 🖥️ **Web Admin Panel** — Full-featured visual configuration with real-time YAML sync
- 📷 **QR Code Login** — Scan to bind, with guided nickname setup on success
- 💬 **Session Management** — Automatic context and conversation history
- 🐳 **Cross-Platform** — Binary / Docker one-click deploy on Linux / macOS / Windows

## 📸 Screenshots

| Dashboard Overview | Add Agent |
| :---: | :---: |
| ![dashboard](docs/screenshot-dashboard.png) | ![add-agent](docs/screenshot-addagent.png) |

| QR Code Login | Binding Success |
| :---: | :---: |
| ![login](docs/screenshot-login.png) | ![binding](docs/screenshot-bindingsuccess.png) |

| Routing Rules | Multi-Account Management |
| :---: | :---: |
| ![routing](docs/screenshot-routing.png) | ![accounts](docs/screenshot-accounts.png) |

## 🚀 Quick Start

### Docker (Recommended)

```bash
# 1. Create config file
curl -o config.yaml https://raw.githubusercontent.com/amigoer/weclaw-proxy/main/configs/config.example.yaml
# Edit config.yaml with your Agent API Key

# 2. Start
docker run -d \
  --name weclaw-proxy \
  -v ./config.yaml:/data/config.yaml \
  -p 8080:8080 \
  ghcr.io/amigoer/weclaw-proxy:latest

# 3. Open http://localhost:8080 to scan QR code and log in
```

### Binary

Download from [Releases](https://github.com/amigoer/weclaw-proxy/releases):

```bash
chmod +x weclaw-proxy-linux-amd64
./weclaw-proxy-linux-amd64 --config config.yaml
```

### Build from Source

```bash
git clone https://github.com/amigoer/weclaw-proxy.git
cd weclaw-proxy
make        # Build frontend + Go binary
make dev    # Run in development mode
```

## ⚙️ Configuration

```yaml
server:
  port: 8080

adapters:
  - name: "openai-gpt4"
    type: openai
    api_key: "sk-xxx"
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"
    system_prompt: "You are a friendly WeChat assistant"

routing:
  default_adapter: "openai-gpt4"
  rules:
    - match:
        prefix: "/claude"
      adapter: "claude"

# Smart Routing (optional)
smart_routing:
  enabled: false
  api_key: "sk-xxx"
  model: "gpt-4o-mini"
```

> 💡 All configuration can be edited live in the Web Admin Panel — no need to manually modify YAML files.

### CLI Agent

Directly invoke locally installed AI CLI tools with automatic sub-command inference:

```yaml
adapters:
  - name: "codex"
    type: cli
    base_url: "codex"       # Command path
    extra:
      timeout: "120"        # Timeout in seconds, default 120
      # args: "exec"        # Optional, auto-inferred
      # work_dir: "/project" # Optional, working directory

  - name: "claude"
    type: cli
    base_url: "claude"      # claude -p "message"

  - name: "gemini"
    type: cli
    base_url: "gemini"      # gemini -p "message"
```

| CLI Tool | Command | Auto-Inferred Args |
|----------|---------|:------------------:|
| Codex | `codex` | `exec` |
| Claude Code | `claude` | `-p` |
| Gemini CLI | `gemini` | `-p` |

## 📦 Supported Platforms

| Platform | AMD64 | ARM64 |
| -------- | :---: | :---: |
| Linux    |   ✅   |   ✅   |
| macOS    |   ✅   |   ✅   |
| Windows  |   ✅   |   ✅   |
| Docker   |   ✅   |   ✅   |

## 🔗 Links

- [Linux.do](https://linux.do) — Open Source Tech Community

## 📄 License

MIT
