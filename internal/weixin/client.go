package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	// 默认 API 基础地址
	DefaultBaseURL = "https://ilinkai.weixin.qq.com"
	// 默认 CDN 基础地址
	DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	// Channel 版本标识
	channelVersion = "weclaw-proxy-go/1.0.0"

	// 默认超时时间
	defaultLongPollTimeoutMs = 35000
	defaultAPITimeoutMs      = 15000
	defaultConfigTimeoutMs   = 10000
)

// Client 微信 ilink API 客户端
type Client struct {
	baseURL           string
	cdnBaseURL        string
	token             string
	httpClient        *http.Client
	longPollTimeoutMs int
	apiTimeoutMs      int
	routeTag          string
	logger            *slog.Logger
}

// ClientOption 客户端配置选项
type ClientOption func(*Client)

// WithToken 设置认证 Token
func WithToken(token string) ClientOption {
	return func(c *Client) { c.token = token }
}

// WithBaseURL 设置 API 基础地址
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) { c.baseURL = baseURL }
}

// WithCDNBaseURL 设置 CDN 基础地址
func WithCDNBaseURL(cdnBaseURL string) ClientOption {
	return func(c *Client) { c.cdnBaseURL = cdnBaseURL }
}

// WithLongPollTimeout 设置长轮询超时时间（毫秒）
func WithLongPollTimeout(ms int) ClientOption {
	return func(c *Client) { c.longPollTimeoutMs = ms }
}

// WithRouteTag 设置路由标签
func WithRouteTag(tag string) ClientOption {
	return func(c *Client) { c.routeTag = tag }
}

// WithLogger 设置日志记录器
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) { c.logger = logger }
}

// NewClient 创建新的微信 API 客户端
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		baseURL:           DefaultBaseURL,
		cdnBaseURL:        DefaultCDNBaseURL,
		longPollTimeoutMs: defaultLongPollTimeoutMs,
		apiTimeoutMs:      defaultAPITimeoutMs,
		httpClient:        &http.Client{},
		logger:            slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetToken 动态更新 Token（登录后调用）
func (c *Client) SetToken(token string) {
	c.token = token
}

// SetBaseURL 动态更新基础地址
func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL = baseURL
}

// buildBaseInfo 构建请求元数据
func buildBaseInfo() *BaseInfo {
	return &BaseInfo{ChannelVersion: channelVersion}
}

// randomWechatUIN 生成 X-WECHAT-UIN 请求头值
// 规则：随机 uint32 → 十进制字符串 → Base64 编码
func randomWechatUIN() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	uint32Val := binary.BigEndian.Uint32(b)
	decStr := fmt.Sprintf("%d", uint32Val)
	return base64.StdEncoding.EncodeToString([]byte(decStr))
}

// buildHeaders 构建请求头
func (c *Client) buildHeaders(bodyLen int) http.Header {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("AuthorizationType", "ilink_bot_token")
	headers.Set("Content-Length", fmt.Sprintf("%d", bodyLen))
	headers.Set("X-WECHAT-UIN", randomWechatUIN())

	if c.token != "" {
		headers.Set("Authorization", "Bearer "+strings.TrimSpace(c.token))
	}
	if c.routeTag != "" {
		headers.Set("SKRouteTag", c.routeTag)
	}
	return headers
}

// ensureTrailingSlash 确保 URL 以 / 结尾
func ensureTrailingSlash(url string) string {
	if strings.HasSuffix(url, "/") {
		return url
	}
	return url + "/"
}

// apiFetch 通用 API 请求方法
func (c *Client) apiFetch(ctx context.Context, endpoint string, body []byte, timeoutMs int, label string) ([]byte, error) {
	base := ensureTrailingSlash(c.baseURL)
	url := base + endpoint

	c.logger.Debug("API 请求",
		"label", label,
		"url", url,
		"bodyLen", len(body),
	)

	// 创建带超时的 context
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%s: 创建请求失败: %w", label, err)
	}

	// 设置请求头
	for k, v := range c.buildHeaders(len(body)) {
		req.Header[k] = v
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s: 请求失败: %w", label, err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: 读取响应失败: %w", label, err)
	}

	c.logger.Debug("API 响应",
		"label", label,
		"status", resp.StatusCode,
		"bodyLen", len(rawBody),
	)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d: %s", label, resp.StatusCode, string(rawBody))
	}

	return rawBody, nil
}

// GetUpdates 长轮询获取新消息
func (c *Client) GetUpdates(ctx context.Context, getUpdatesBuf string) (*GetUpdatesResp, error) {
	reqBody := &GetUpdatesReq{
		GetUpdatesBuf: getUpdatesBuf,
		BaseInfo:      buildBaseInfo(),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("getUpdates: 序列化请求失败: %w", err)
	}

	rawResp, err := c.apiFetch(ctx, "ilink/bot/getupdates", body, c.longPollTimeoutMs, "getUpdates")
	if err != nil {
		// 长轮询超时是正常行为，返回空响应
		if ctx.Err() == context.DeadlineExceeded {
			c.logger.Debug("getUpdates: 客户端超时，返回空响应")
			return &GetUpdatesResp{
				Ret:           0,
				Msgs:          nil,
				GetUpdatesBuf: getUpdatesBuf,
			}, nil
		}
		return nil, err
	}

	var resp GetUpdatesResp
	if err := json.Unmarshal(rawResp, &resp); err != nil {
		return nil, fmt.Errorf("getUpdates: 解析响应失败: %w", err)
	}

	return &resp, nil
}

// SendMessage 发送消息
func (c *Client) SendMessage(ctx context.Context, req *SendMessageReq) error {
	if req.BaseInfo == nil {
		req.BaseInfo = buildBaseInfo()
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("sendMessage: 序列化请求失败: %w", err)
	}

	_, err = c.apiFetch(ctx, "ilink/bot/sendmessage", body, c.apiTimeoutMs, "sendMessage")
	return err
}

// GetUploadUrl 获取 CDN 预签名上传 URL
func (c *Client) GetUploadUrl(ctx context.Context, req *GetUploadUrlReq) (*GetUploadUrlResp, error) {
	if req.BaseInfo == nil {
		req.BaseInfo = buildBaseInfo()
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("getUploadUrl: 序列化请求失败: %w", err)
	}

	rawResp, err := c.apiFetch(ctx, "ilink/bot/getuploadurl", body, c.apiTimeoutMs, "getUploadUrl")
	if err != nil {
		return nil, err
	}

	var resp GetUploadUrlResp
	if err := json.Unmarshal(rawResp, &resp); err != nil {
		return nil, fmt.Errorf("getUploadUrl: 解析响应失败: %w", err)
	}

	return &resp, nil
}

// GetConfig 获取 Bot 配置
func (c *Client) GetConfig(ctx context.Context, userID string, contextToken string) (*GetConfigResp, error) {
	reqBody := &GetConfigReq{
		ILinkUserID:  userID,
		ContextToken: contextToken,
		BaseInfo:     buildBaseInfo(),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("getConfig: 序列化请求失败: %w", err)
	}

	rawResp, err := c.apiFetch(ctx, "ilink/bot/getconfig", body, defaultConfigTimeoutMs, "getConfig")
	if err != nil {
		return nil, err
	}

	var resp GetConfigResp
	if err := json.Unmarshal(rawResp, &resp); err != nil {
		return nil, fmt.Errorf("getConfig: 解析响应失败: %w", err)
	}

	return &resp, nil
}

// SendTyping 发送输入状态指示器
func (c *Client) SendTyping(ctx context.Context, req *SendTypingReq) error {
	if req.BaseInfo == nil {
		req.BaseInfo = buildBaseInfo()
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("sendTyping: 序列化请求失败: %w", err)
	}

	_, err = c.apiFetch(ctx, "ilink/bot/sendtyping", body, defaultConfigTimeoutMs, "sendTyping")
	return err
}
