package weixin

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
)

// Sender 消息发送器
type Sender struct {
	client *Client
	logger *slog.Logger
}

// NewSender 创建新的消息发送器
func NewSender(client *Client, logger *slog.Logger) *Sender {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sender{
		client: client,
		logger: logger,
	}
}

// generateClientID 生成唯一的客户端消息 ID
func generateClientID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("weclaw-%x", b)
}

// SendText 发送文本消息
func (s *Sender) SendText(ctx context.Context, to string, text string, contextToken string) (string, error) {
	clientID := generateClientID()

	var itemList []*MessageItem
	if text != "" {
		itemList = []*MessageItem{
			{
				Type:     MessageItemTypeText,
				TextItem: &TextItem{Text: text},
			},
		}
	}

	req := &SendMessageReq{
		Msg: &WeixinMessage{
			FromUserID:   "",
			ToUserID:     to,
			ClientID:     clientID,
			MessageType:  MessageTypeBot,
			MessageState: MessageStateFinish,
			ItemList:     itemList,
			ContextToken: contextToken,
		},
	}

	s.logger.Info("发送文本消息",
		"to", to,
		"clientID", clientID,
		"textLen", len(text),
	)

	if err := s.client.SendMessage(ctx, req); err != nil {
		s.logger.Error("发送文本消息失败",
			"to", to,
			"error", err,
		)
		return "", err
	}

	return clientID, nil
}

// SendTypingIndicator 发送"正在输入"状态
func (s *Sender) SendTypingIndicator(ctx context.Context, userID string, typingTicket string, typing bool) error {
	status := TypingStatusTyping
	if !typing {
		status = TypingStatusCancel
	}

	req := &SendTypingReq{
		ILinkUserID:  userID,
		TypingTicket: typingTicket,
		Status:       status,
	}

	return s.client.SendTyping(ctx, req)
}
