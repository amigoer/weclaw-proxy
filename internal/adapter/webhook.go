package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// 通用 Webhook 适配器
// 将消息 POST 到自定义 URL，解析 JSON 响应
// 支持接入任何实现了对应 JSON 接口的外部 Agent

// webhookRequest Webhook 请求格式
type webhookRequest struct {
	UserID    string         `json:"user_id"`
	Message   string         `json:"message"`
	SessionID string         `json:"session_id,omitempty"`
	History   []HistoryEntry `json:"history,omitempty"`
	MediaURL  string         `json:"media_url,omitempty"`
}

// webhookResponse Webhook 响应格式
type webhookResponse struct {
	Text      string   `json:"text"`
	MediaURLs []string `json:"media_urls,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// WebhookAdapter 通用 Webhook 适配器
type WebhookAdapter struct {
	name       string
	url        string
	apiKey     string
	httpClient *http.Client
	headers    map[string]string
	logger     *slog.Logger
}

// NewWebhookAdapter 创建 Webhook 适配器
func NewWebhookAdapter(cfg *AdapterConfig, logger *slog.Logger) *WebhookAdapter {
	if logger == nil {
		logger = slog.Default()
	}

	url := cfg.BaseURL
	if url == "" {
		url = cfg.Extra["url"]
	}

	headers := make(map[string]string)
	// 从 extra 中提取自定义 header
	for k, v := range cfg.Extra {
		if len(k) > 7 && k[:7] == "header_" {
			headers[k[7:]] = v
		}
	}

	return &WebhookAdapter{
		name:       cfg.Name,
		url:        url,
		apiKey:     cfg.APIKey,
		httpClient: &http.Client{},
		headers:    headers,
		logger:     logger,
	}
}

func (a *WebhookAdapter) Name() string { return a.name }
func (a *WebhookAdapter) Type() string { return "webhook" }

// Chat 同步对话
func (a *WebhookAdapter) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	webhookReq := &webhookRequest{
		UserID:    req.UserID,
		Message:   req.Message,
		SessionID: req.SessionID,
		History:   req.History,
		MediaURL:  req.MediaURL,
	}

	body, err := json.Marshal(webhookReq)
	if err != nil {
		return nil, fmt.Errorf("webhook: 序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webhook: 创建请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	for k, v := range a.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("webhook: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("webhook: 读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("webhook: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var webhookResp webhookResponse
	if err := json.Unmarshal(rawBody, &webhookResp); err != nil {
		return nil, fmt.Errorf("webhook: 解析响应失败: %w", err)
	}

	if webhookResp.Error != "" {
		return nil, fmt.Errorf("webhook: 服务端错误: %s", webhookResp.Error)
	}

	return &ChatResponse{
		Text:      webhookResp.Text,
		MediaURLs: webhookResp.MediaURLs,
	}, nil
}

// ChatStream Webhook 适配器不支持流式，回退到同步模式
func (a *WebhookAdapter) ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error) {
	resp, err := a.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *ChatChunk, 1)
	go func() {
		defer close(ch)
		ch <- &ChatChunk{
			Text: resp.Text,
			Done: true,
		}
	}()

	return ch, nil
}
