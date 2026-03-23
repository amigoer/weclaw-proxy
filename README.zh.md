# WeClaw-Proxy

[English](README.md)

微信 Agent 代理适配器 —— 将外部 AI Agent 无缝接入微信。

## 功能特性

- 🔌 **多 Agent 适配** — 内置 OpenAI 兼容适配器（支持 GPT-4、vLLM、Ollama 等）和通用 Webhook 适配器
- 🔀 **智能路由** — 按消息前缀命令或用户 ID 路由到不同 Agent
- 💬 **会话管理** — 自动维护对话上下文和历史记录
- 📱 **扫码登录** — 微信二维码一键连接
- ⚡ **长轮询** — 基于微信 ilink 协议的实时消息收发
- 🔄 **断线恢复** — 同步游标持久化，重启后无缝续接
- 🖥️ **管理后台** — 基于 React + shadcn/ui 的可视化管理面板

## 快速开始

### 1. 编译

```bash
make          # 构建前端 + Go 二进制
# 或手动：
# cd web && npm install && npm run build
# go build -o weclaw-proxy ./cmd/weclaw-proxy/
```

### 2. 配置

```bash
cp configs/config.example.yaml configs/config.yaml
# 编辑 config.yaml，填入你的 Agent API Key
```

### 3. 登录微信

```bash
make login
# 或: ./weclaw-proxy --login --config configs/config.yaml
# 用微信扫描终端显示的二维码
```

### 4. 启动服务

```bash
export OPENAI_API_KEY=sk-xxx
make dev
# 或: ./weclaw-proxy --config configs/config.yaml
```

启动后访问 `http://localhost:8080` 进入管理面板。

## Make 命令

| 命令 | 说明 |
|------|------|
| `make` | 构建前端 + Go 二进制 |
| `make web` | 仅构建前端 |
| `make go` | 仅构建 Go 后端 |
| `make dev` | 启动开发模式 |
| `make dev-web` | 启动前端热更新服务器 |
| `make login` | 微信扫码登录 |
| `make lint` | 代码检查 |
| `make clean` | 清理构建产物 |

## 管理后台

内置 Web 管理面板，无需编辑 YAML 文件即可在线管理：

- **仪表盘** — 连接状态、Agent 数量、活跃会话
- **Agent 管理** — 在线添加/编辑/删除 Agent，即时生效
- **路由规则** — 可视化配置前缀匹配路由规则

管理面板基于 React + shadcn/ui 构建，通过 `go:embed` 嵌入 Go 二进制，无需额外部署。

## 配置说明

### Agent 适配器

支持的适配器类型：

| 类型 | 说明 | 配置项 |
|------|------|--------|
| `openai` | OpenAI ChatCompletion 兼容 | `api_key`, `base_url`, `model`, `system_prompt` |
| `webhook` | 通用 Webhook 转发 | `base_url`, `api_key` |

任何提供 HTTP 端点的 Agent 均可通过 `webhook` 类型接入。

### 路由规则

```yaml
routing:
  default_adapter: "openai-gpt4"
  rules:
    # 按前缀匹配：发送 "/claude 你好" 会路由到 claude 适配器
    - match:
        prefix: "/claude"
      adapter: "claude"
    # 按用户 ID 匹配
    - match:
        user_ids: ["user@im.wechat"]
      adapter: "my-dify-bot"
```

### 内置命令

| 命令 | 功能 |
|------|------|
| `/clear` | 清除对话历史 |
| `/reset` | 重置对话上下文 |
| `/help` | 显示帮助信息 |

## 项目结构

```
weclaw-proxy/
├── cmd/weclaw-proxy/main.go     # 程序入口
├── internal/
│   ├── weixin/                   # 微信 ilink 协议实现
│   │   ├── types.go              # 协议类型定义
│   │   ├── client.go             # API 客户端
│   │   ├── auth.go               # 二维码登录
│   │   ├── poller.go             # 长轮询消息监听
│   │   └── sender.go             # 消息发送
│   ├── adapter/                  # Agent 适配器
│   │   ├── adapter.go            # 接口定义
│   │   ├── openai.go             # OpenAI 适配器
│   │   └── webhook.go            # Webhook 适配器
│   ├── server/                   # 管理后台
│   │   ├── api.go                # REST API
│   │   ├── store.go              # 运行时配置存储
│   │   └── embed.go              # 前端资源嵌入
│   ├── router/router.go          # 消息路由
│   ├── session/session.go        # 会话管理
│   └── config/config.go          # 配置管理
├── web/                          # 管理面板前端（React + shadcn/ui）
├── configs/config.example.yaml   # 示例配置
├── Makefile                      # 构建命令
└── go.mod
```

## 协议原理

本项目基于微信 ilink 协议（反解析自 `@tencent-weixin/openclaw-weixin` v1.0.3），核心 API：

- `POST /ilink/bot/getupdates` — 长轮询接收消息
- `POST /ilink/bot/sendmessage` — 发送消息
- `POST /ilink/bot/getconfig` — 获取配置
- `POST /ilink/bot/sendtyping` — 输入状态指示器
- `GET /ilink/bot/get_bot_qrcode` — 获取登录二维码

## License

MIT
