package weixin

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	// 连续失败最大次数
	maxConsecutiveFailures = 3
	// 退避延迟（连续失败 3 次后）
	backoffDelayMs = 30000
	// 重试延迟（单次失败）
	retryDelayMs = 2000
	// session 过期错误码
	sessionExpiredErrCode = -14
	// session 过期暂停时间
	sessionPauseMs = 300000 // 5 分钟
)

// MessageHandler 消息处理回调
type MessageHandler func(msg *WeixinMessage)

// Poller 消息长轮询器
type Poller struct {
	client         *Client
	accountID      string
	handler        MessageHandler
	getUpdatesBuf  string // 同步游标
	logger         *slog.Logger
	onBufUpdate    func(buf string) // 同步游标更新回调（用于持久化）
}

// PollerOption 长轮询器配置选项
type PollerOption func(*Poller)

// WithPollerHandler 设置消息处理回调
func WithPollerHandler(handler MessageHandler) PollerOption {
	return func(p *Poller) { p.handler = handler }
}

// WithPollerAccountID 设置账号 ID
func WithPollerAccountID(accountID string) PollerOption {
	return func(p *Poller) { p.accountID = accountID }
}

// WithPollerLogger 设置日志记录器
func WithPollerLogger(logger *slog.Logger) PollerOption {
	return func(p *Poller) { p.logger = logger }
}

// WithInitialSyncBuf 设置初始同步游标（用于恢复）
func WithInitialSyncBuf(buf string) PollerOption {
	return func(p *Poller) { p.getUpdatesBuf = buf }
}

// WithBufUpdateCallback 设置同步游标更新回调
func WithBufUpdateCallback(cb func(buf string)) PollerOption {
	return func(p *Poller) { p.onBufUpdate = cb }
}

// NewPoller 创建新的消息长轮询器
func NewPoller(client *Client, opts ...PollerOption) *Poller {
	p := &Poller{
		client: client,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Start 启动长轮询循环（阻塞，直到 ctx 被取消）
func (p *Poller) Start(ctx context.Context) error {
	p.logger.Info("消息轮询已启动",
		"accountID", p.accountID,
		"baseURL", p.client.baseURL,
	)

	consecutiveFailures := 0

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("消息轮询已停止", "accountID", p.accountID)
			return ctx.Err()
		default:
		}

		resp, err := p.client.GetUpdates(ctx, p.getUpdatesBuf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			consecutiveFailures++
			p.logger.Error("getUpdates 请求失败",
				"error", err,
				"failures", consecutiveFailures,
				"max", maxConsecutiveFailures,
			)

			if consecutiveFailures >= maxConsecutiveFailures {
				p.logger.Error("连续失败过多，退避等待",
					"delayMs", backoffDelayMs,
				)
				consecutiveFailures = 0
				p.sleep(ctx, backoffDelayMs)
			} else {
				p.sleep(ctx, retryDelayMs)
			}
			continue
		}

		// 检查 API 错误
		isAPIError := (resp.Ret != 0) || (resp.ErrCode != 0)
		if isAPIError {
			// 检查 session 过期
			if resp.ErrCode == sessionExpiredErrCode || resp.Ret == sessionExpiredErrCode {
				p.logger.Error("session 已过期，暂停重连",
					"errcode", resp.ErrCode,
					"pauseMs", sessionPauseMs,
				)
				consecutiveFailures = 0
				p.sleep(ctx, sessionPauseMs)
				continue
			}

			consecutiveFailures++
			p.logger.Error("getUpdates API 错误",
				"ret", resp.Ret,
				"errcode", resp.ErrCode,
				"errmsg", resp.ErrMsg,
				"failures", consecutiveFailures,
			)

			if consecutiveFailures >= maxConsecutiveFailures {
				consecutiveFailures = 0
				p.sleep(ctx, backoffDelayMs)
			} else {
				p.sleep(ctx, retryDelayMs)
			}
			continue
		}

		// 请求成功，重置失败计数
		consecutiveFailures = 0

		// 更新同步游标
		if resp.GetUpdatesBuf != "" {
			p.getUpdatesBuf = resp.GetUpdatesBuf
			if p.onBufUpdate != nil {
				p.onBufUpdate(resp.GetUpdatesBuf)
			}
		}

		// 处理收到的消息
		for _, msg := range resp.Msgs {
			if msg == nil {
				continue
			}
			p.logger.Info("收到新消息",
				"from", msg.FromUserID,
				"itemCount", len(msg.ItemList),
			)
			if p.handler != nil {
				p.handler(msg)
			}
		}
	}
}

// GetSyncBuf 获取当前同步游标（用于持久化）
func (p *Poller) GetSyncBuf() string {
	return p.getUpdatesBuf
}

// sleep 带 context 取消支持的延迟
func (p *Poller) sleep(ctx context.Context, ms int) {
	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(ms) * time.Millisecond):
	}
}

// ExtractTextFromMessage 从消息中提取文本内容
func ExtractTextFromMessage(msg *WeixinMessage) string {
	if msg == nil || len(msg.ItemList) == 0 {
		return ""
	}

	for _, item := range msg.ItemList {
		if item.Type == MessageItemTypeText && item.TextItem != nil && item.TextItem.Text != "" {
			text := item.TextItem.Text

			// 处理引用消息
			if item.RefMsg != nil {
				ref := item.RefMsg
				if ref.MessageItem != nil && isMediaItem(ref.MessageItem) {
					// 引用的是媒体消息，只返回当前文本
					return text
				}
				var parts []string
				if ref.Title != "" {
					parts = append(parts, ref.Title)
				}
				if ref.MessageItem != nil {
					refText := extractTextFromItem(ref.MessageItem)
					if refText != "" {
						parts = append(parts, refText)
					}
				}
				if len(parts) > 0 {
					return fmt.Sprintf("[引用: %s]\n%s", joinStrings(parts, " | "), text)
				}
			}
			return text
		}

		// 语音转文字
		if item.Type == MessageItemTypeVoice && item.VoiceItem != nil && item.VoiceItem.Text != "" {
			return item.VoiceItem.Text
		}
	}

	return ""
}

// isMediaItem 判断是否为媒体类型消息项
func isMediaItem(item *MessageItem) bool {
	return item.Type == MessageItemTypeImage ||
		item.Type == MessageItemTypeVideo ||
		item.Type == MessageItemTypeFile ||
		item.Type == MessageItemTypeVoice
}

// extractTextFromItem 从单个消息项提取文本
func extractTextFromItem(item *MessageItem) string {
	if item.Type == MessageItemTypeText && item.TextItem != nil {
		return item.TextItem.Text
	}
	return ""
}

// joinStrings 连接字符串切片
func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
