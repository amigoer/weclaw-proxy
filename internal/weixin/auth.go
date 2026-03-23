package weixin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// 默认 bot_type（来自源码 DEFAULT_ILINK_BOT_TYPE = "3"）
	DefaultBotType = "3"
	// 二维码状态轮询超时
	qrPollTimeoutMs = 35000
	// 二维码登录最大等待时间
	defaultLoginTimeoutMs = 480000
	// 二维码最大刷新次数
	maxQRRefreshCount = 3
)

// LoginResult 登录结果
type LoginResult struct {
	Connected bool   // 是否连接成功
	BotToken  string // Bot Token
	AccountID string // ilink_bot_id（归一化前的原始 ID）
	BaseURL   string // API 基础地址
	UserID    string // 扫码用户 ID
	Message   string // 状态消息
}

// QRCodeInfo 二维码信息
type QRCodeInfo struct {
	QRCodeURL  string // 二维码图片 URL（供用户扫描）
	QRCode     string // 二维码原始值
	SessionKey string // 会话标识
}

// AuthClient 认证客户端
type AuthClient struct {
	baseURL string
	logger  *slog.Logger
}

// NewAuthClient 创建新的认证客户端
func NewAuthClient(baseURL string, logger *slog.Logger) *AuthClient {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthClient{
		baseURL: baseURL,
		logger:  logger,
	}
}

// GetBaseURL 返回认证服务的基础 URL
func (a *AuthClient) GetBaseURL() string {
	return a.baseURL
}

// FetchQRCode 获取登录二维码
func (a *AuthClient) FetchQRCode(ctx context.Context, botType string) (*QRCodeInfo, error) {
	if botType == "" {
		botType = DefaultBotType
	}

	base := ensureTrailingSlash(a.baseURL)
	reqURL := fmt.Sprintf("%silink/bot/get_bot_qrcode?bot_type=%s", base, url.QueryEscape(botType))

	a.logger.Info("正在获取登录二维码", "url", reqURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建二维码请求失败: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取二维码失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取二维码失败: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var qrResp QRCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&qrResp); err != nil {
		return nil, fmt.Errorf("解析二维码响应失败: %w", err)
	}

	a.logger.Info("二维码获取成功")

	return &QRCodeInfo{
		QRCodeURL: qrResp.QRCodeImgContent,
		QRCode:    qrResp.QRCode,
	}, nil
}

// PollQRStatus 轮询二维码扫码状态（单次请求）
func (a *AuthClient) PollQRStatus(ctx context.Context, qrcode string) (*QRStatusResponse, error) {
	base := ensureTrailingSlash(a.baseURL)
	reqURL := fmt.Sprintf("%silink/bot/get_qrcode_status?qrcode=%s", base, url.QueryEscape(qrcode))

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(qrPollTimeoutMs)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建状态请求失败: %w", err)
	}
	req.Header.Set("iLink-App-ClientVersion", "1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// 超时是正常的长轮询行为
		if ctx.Err() == context.DeadlineExceeded || reqCtx.Err() == context.DeadlineExceeded {
			return &QRStatusResponse{Status: "wait"}, nil
		}
		return nil, fmt.Errorf("轮询状态失败: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取状态响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("轮询状态失败: HTTP %d: %s", resp.StatusCode, string(rawBody))
	}

	var statusResp QRStatusResponse
	if err := json.Unmarshal(rawBody, &statusResp); err != nil {
		return nil, fmt.Errorf("解析状态响应失败: %w", err)
	}

	return &statusResp, nil
}

// Login 执行完整的二维码登录流程
// onQRCode 回调用于展示二维码给用户
// onStatus 回调用于通知状态变更
func (a *AuthClient) Login(ctx context.Context, onQRCode func(qr *QRCodeInfo), onStatus func(msg string)) (*LoginResult, error) {
	// 获取初始二维码
	qrInfo, err := a.FetchQRCode(ctx, DefaultBotType)
	if err != nil {
		return &LoginResult{Connected: false, Message: err.Error()}, err
	}

	if onQRCode != nil {
		onQRCode(qrInfo)
	}

	loginTimeout := time.Duration(defaultLoginTimeoutMs) * time.Millisecond
	deadline := time.Now().Add(loginTimeout)
	scannedPrinted := false
	qrRefreshCount := 1
	currentQRCode := qrInfo.QRCode

	if onStatus != nil {
		onStatus("等待微信扫码...")
	}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return &LoginResult{Connected: false, Message: "登录已取消"}, ctx.Err()
		default:
		}

		statusResp, err := a.PollQRStatus(ctx, currentQRCode)
		if err != nil {
			return &LoginResult{Connected: false, Message: err.Error()}, err
		}

		switch statusResp.Status {
		case "wait":
			// 继续等待
			continue

		case "scaned":
			if !scannedPrinted {
				scannedPrinted = true
				a.logger.Info("用户已扫码，等待确认")
				if onStatus != nil {
					onStatus("👀 已扫码，请在微信中确认...")
				}
			}

		case "expired":
			qrRefreshCount++
			if qrRefreshCount > maxQRRefreshCount {
				msg := "二维码多次过期，请重新开始登录流程"
				return &LoginResult{Connected: false, Message: msg}, errors.New(msg)
			}

			a.logger.Info("二维码已过期，正在刷新",
				"count", qrRefreshCount,
				"max", maxQRRefreshCount,
			)
			if onStatus != nil {
				onStatus(fmt.Sprintf("⏳ 二维码已过期，正在刷新...(%d/%d)", qrRefreshCount, maxQRRefreshCount))
			}

			newQR, err := a.FetchQRCode(ctx, DefaultBotType)
			if err != nil {
				return &LoginResult{Connected: false, Message: err.Error()}, err
			}
			currentQRCode = newQR.QRCode
			scannedPrinted = false

			if onQRCode != nil {
				onQRCode(newQR)
			}

		case "confirmed":
			if statusResp.ILinkBotID == "" {
				msg := "登录失败：服务器未返回 ilink_bot_id"
				return &LoginResult{Connected: false, Message: msg}, errors.New(msg)
			}

			a.logger.Info("✅ 登录成功",
				"botID", statusResp.ILinkBotID,
				"baseURL", statusResp.BaseURL,
			)

			baseURL := statusResp.BaseURL
			if baseURL == "" {
				baseURL = a.baseURL
			}

			return &LoginResult{
				Connected: true,
				BotToken:  statusResp.BotToken,
				AccountID: statusResp.ILinkBotID,
				BaseURL:   baseURL,
				UserID:    statusResp.ILinkUserID,
				Message:   "✅ 与微信连接成功！",
			}, nil
		}

		time.Sleep(1 * time.Second)
	}

	msg := "登录超时，请重试"
	return &LoginResult{Connected: false, Message: msg}, errors.New(msg)
}

// NormalizeAccountID 将原始 ilink_bot_id 归一化为文件系统安全的 key
// 例如: "b0f5860fdecb@im.bot" → "b0f5860fdecb-im-bot"
func NormalizeAccountID(rawID string) string {
	result := strings.ReplaceAll(rawID, "@", "-")
	result = strings.ReplaceAll(result, ".", "-")
	return result
}
