package adapter

import "context"

// HistoryEntry 对话历史条目
type HistoryEntry struct {
	Role    string `json:"role"`    // "user" 或 "assistant"
	Content string `json:"content"` // 消息内容
}

// ChatRequest Agent 对话请求
type ChatRequest struct {
	UserID    string         `json:"user_id"`              // 微信用户 ID
	Message   string         `json:"message"`              // 用户消息文本
	MediaURL  string         `json:"media_url,omitempty"`  // 媒体文件路径（可选）
	History   []HistoryEntry `json:"history,omitempty"`    // 上下文历史
	SessionID string         `json:"session_id,omitempty"` // 会话 ID
}

// ChatResponse Agent 对话响应
type ChatResponse struct {
	Text      string   `json:"text"`                 // 回复文本
	MediaURLs []string `json:"media_urls,omitempty"` // 回复媒体文件（可选）
}

// ChatChunk 流式对话片段
type ChatChunk struct {
	Text  string `json:"text"`            // 文本片段
	Done  bool   `json:"done"`            // 是否是最后一个片段
	Error error  `json:"error,omitempty"` // 错误信息
}

// Adapter Agent 适配器接口
type Adapter interface {
	// Chat 同步对话
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// ChatStream 流式对话（返回片段 channel）
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error)

	// Name 适配器名称
	Name() string

	// Type 适配器类型标识
	Type() string
}

// AdapterConfig 适配器通用配置
type AdapterConfig struct {
	Name         string            `yaml:"name"`                    // 自定义名称
	AdapterType  string            `yaml:"type"`                    // 类型: openai, anthropic, dify, coze, webhook
	APIKey       string            `yaml:"api_key,omitempty"`       // API Key
	BaseURL      string            `yaml:"base_url,omitempty"`      // API 基础地址
	Model        string            `yaml:"model,omitempty"`         // 模型名称
	SystemPrompt string            `yaml:"system_prompt,omitempty"` // 系统提示词
	MaxTokens    int               `yaml:"max_tokens,omitempty"`    // 最大输出 Token 数
	Temperature  float64           `yaml:"temperature,omitempty"`   // 温度参数
	Extra        map[string]string `yaml:"extra,omitempty"`         // 额外参数
}
