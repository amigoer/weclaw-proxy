package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// OpenAI ChatCompletion 兼容适配器
// 支持所有 OpenAI API 格式的服务（包括 vLLM、Ollama、Azure OpenAI 等）

// openAIMessage OpenAI 消息格式
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatRequest OpenAI ChatCompletion 请求
type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

// openAIChatResponse OpenAI ChatCompletion 响应
type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// openAIStreamDelta SSE 流式增量数据
type openAIStreamDelta struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// OpenAIAdapter OpenAI 兼容适配器
type OpenAIAdapter struct {
	name         string
	baseURL      string
	apiKey       string
	model        string
	systemPrompt string
	maxTokens    int
	temperature  float64
	httpClient   *http.Client
	logger       *slog.Logger
}

// NewOpenAIAdapter 创建 OpenAI 适配器
func NewOpenAIAdapter(cfg *AdapterConfig, logger *slog.Logger) *OpenAIAdapter {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	// 确保不以 / 结尾
	baseURL = strings.TrimRight(baseURL, "/")

	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	temperature := cfg.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &OpenAIAdapter{
		name:         cfg.Name,
		baseURL:      baseURL,
		apiKey:       cfg.APIKey,
		model:        model,
		systemPrompt: cfg.SystemPrompt,
		maxTokens:    maxTokens,
		temperature:  temperature,
		httpClient:   &http.Client{},
		logger:       logger,
	}
}

func (a *OpenAIAdapter) Name() string { return a.name }
func (a *OpenAIAdapter) Type() string { return "openai" }

// buildMessages 构建 OpenAI 消息列表
func (a *OpenAIAdapter) buildMessages(req *ChatRequest) []openAIMessage {
	var messages []openAIMessage

	// 系统提示词
	if a.systemPrompt != "" {
		messages = append(messages, openAIMessage{
			Role:    "system",
			Content: a.systemPrompt,
		})
	}

	// 历史消息
	for _, h := range req.History {
		messages = append(messages, openAIMessage{
			Role:    h.Role,
			Content: h.Content,
		})
	}

	// 当前用户消息
	messages = append(messages, openAIMessage{
		Role:    "user",
		Content: req.Message,
	})

	return messages
}

// Chat 同步对话
func (a *OpenAIAdapter) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	chatReq := &openAIChatRequest{
		Model:       a.model,
		Messages:    a.buildMessages(req),
		Stream:      false,
		MaxTokens:   a.maxTokens,
		Temperature: a.temperature,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai: 序列化请求失败: %w", err)
	}

	url := a.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: 创建请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: 读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(rawBody, &chatResp); err != nil {
		return nil, fmt.Errorf("openai: 解析响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: 响应中没有 choices")
	}

	return &ChatResponse{
		Text: chatResp.Choices[0].Message.Content,
	}, nil
}

// ChatStream 流式对话
func (a *OpenAIAdapter) ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error) {
	chatReq := &openAIChatRequest{
		Model:       a.model,
		Messages:    a.buildMessages(req),
		Stream:      true,
		MaxTokens:   a.maxTokens,
		Temperature: a.temperature,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai: 序列化请求失败: %w", err)
	}

	url := a.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: 创建请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: 请求失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	ch := make(chan *ChatChunk, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// SSE 格式: "data: {...}" 或 "data: [DONE]"
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- &ChatChunk{Done: true}
				return
			}

			var delta openAIStreamDelta
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				a.logger.Warn("openai: 解析 SSE 数据失败",
					"data", data,
					"error", err,
				)
				continue
			}

			if len(delta.Choices) > 0 && delta.Choices[0].Delta.Content != "" {
				ch <- &ChatChunk{
					Text: delta.Choices[0].Delta.Content,
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- &ChatChunk{Error: err}
		}
	}()

	return ch, nil
}
