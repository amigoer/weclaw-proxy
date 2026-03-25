# WeClaw-Proxy

> 微信开放平台 AI Agent 代理适配器 —— 让任意 AI Agent 一键对接微信

[English](README.md)

## ✨ 特性

- 🔌 **多 Agent 接入** — 支持 OpenAI、DeepSeek、Ollama、Dify、Coze、Webhook、**本地 CLI**（Codex / Claude Code / Gemini CLI）
- 📱 **多微信账号** — 同时绑定和管理多个微信账号
- 🧠 **智能路由** — LLM 驱动的消息自动分类，也支持前缀 `/command` 手动路由
- 🖥️ **Web 管理面板** — 全功能可视化配置，所有改动实时同步到 YAML
- 📷 **扫码登录** — 网页端扫码绑定，成功后引导设置备注
- 💬 **会话管理** — 自动维护上下文和对话历史
- 🐳 **跨平台部署** — 二进制 / Docker 一键启动，支持 Linux / macOS / Windows

## 📸 截图

| 面板总览 | 添加 Agent |
| :---: | :---: |
| ![dashboard](docs/screenshot-dashboard.png) | ![add-agent](docs/screenshot-addagent.png) |

| 扫码登录 | 绑定成功 |
| :---: | :---: |
| ![login](docs/screenshot-login.png) | ![binding](docs/screenshot-bindingsuccess.png) |

| 路由规则 | 多账号管理 |
| :---: | :---: |
| ![routing](docs/screenshot-routing.png) | ![accounts](docs/screenshot-accounts.png) |

## 🚀 快速开始

### Docker（推荐）

```bash
# 1. 创建配置文件
curl -o config.yaml https://raw.githubusercontent.com/amigoer/weclaw-proxy/main/configs/config.example.yaml
# 编辑 config.yaml，填入你的 Agent API Key

# 2. 启动
docker run -d \
  --name weclaw-proxy \
  -v ./config.yaml:/data/config.yaml \
  -p 8080:8080 \
  ghcr.io/amigoer/weclaw-proxy:latest

# 3. 打开 http://localhost:8080 扫码登录微信
```

### 二进制部署

从 [Releases](https://github.com/amigoer/weclaw-proxy/releases) 下载对应平台的二进制文件：

```bash
chmod +x weclaw-proxy-linux-amd64
./weclaw-proxy-linux-amd64 --config config.yaml
```

### 从源码构建

```bash
git clone https://github.com/amigoer/weclaw-proxy.git
cd weclaw-proxy
make        # 构建前端 + Go 二进制
make dev    # 开发模式运行
```

## ⚙️ 配置示例

```yaml
server:
  port: 8080

adapters:
  - name: "openai-gpt4"
    type: openai
    api_key: "sk-xxx"
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"
    system_prompt: "你是一个友好的微信助手"

routing:
  default_adapter: "openai-gpt4"
  rules:
    - match:
        prefix: "/claude"
      adapter: "claude"

# 智能路由（可选）
smart_routing:
  enabled: false
  api_key: "sk-xxx"
  model: "gpt-4o-mini"
```

> 💡 所有配置都可以在 Web 管理面板中在线编辑，无需手动修改 YAML 文件。

### CLI Agent 配置

支持直接调用本地安装的 AI CLI 工具，自动推断子命令参数：

```yaml
adapters:
  - name: "codex"
    type: cli
    base_url: "codex"       # 命令路径
    extra:
      timeout: "120"        # 超时（秒），默认 120
      # args: "exec"        # 可选，留空自动推断
      # work_dir: "/project" # 可选，工作目录

  - name: "claude"
    type: cli
    base_url: "claude"      # claude -p "消息"

  - name: "gemini"
    type: cli
    base_url: "gemini"      # gemini -p "消息"
```

| CLI 工具 | 命令 | 自动推断参数 |
|----------|------|:----------:|
| Codex    | `codex` | `exec` |
| Claude Code | `claude` | `-p` |
| Gemini CLI | `gemini` | `-p` |

## 📦 支持的平台

| 平台    | AMD64 | ARM64 |
| ------- | :---: | :---: |
| Linux   |   ✅   |   ✅   |
| macOS   |   ✅   |   ✅   |
| Windows |   ✅   |   ✅   |
| Docker  |   ✅   |   ✅   |

## 🔗 友情链接

- [Linux.do](https://linux.do) — 开源技术社区

## 📄 License

MIT
