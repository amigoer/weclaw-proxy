package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/amigoer/weclaw-proxy/internal/adapter"
	"github.com/amigoer/weclaw-proxy/internal/config"
	"github.com/amigoer/weclaw-proxy/internal/router"
	"github.com/amigoer/weclaw-proxy/internal/server"
	"github.com/amigoer/weclaw-proxy/internal/session"
	"github.com/amigoer/weclaw-proxy/internal/weixin"
)

var (
	configPath = flag.String("config", "configs/config.yaml", "配置文件路径")
	loginOnly  = flag.Bool("login", false, "仅执行微信登录，不启动服务")
	verbose    = flag.Bool("verbose", false, "启用详细日志")
)

func main() {
	flag.Parse()

	// 初始化日志
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("加载配置失败", "error", err)
		os.Exit(1)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		logger.Error("配置验证失败", "error", err)
		os.Exit(1)
	}

	logger.Info("配置加载成功",
		"adapters", len(cfg.Adapters),
		"routingRules", len(cfg.Routing.Rules),
	)

	// 创建微信客户端
	wxClient := weixin.NewClient(
		weixin.WithBaseURL(cfg.Weixin.BaseURL),
		weixin.WithCDNBaseURL(cfg.Weixin.CDNBaseURL),
		weixin.WithLongPollTimeout(cfg.Weixin.LongPollTimeoutMs),
		weixin.WithLogger(logger.With("module", "weixin")),
	)

	// 仅登录模式
	if *loginOnly {
		if err := doLogin(cfg, wxClient, logger); err != nil {
			logger.Error("登录失败", "error", err)
			os.Exit(1)
		}
		return
	}

	// 尝试加载已有 Token
	weixinConnected := false
	if !loadSavedToken(cfg, wxClient, logger) {
		logger.Info("未找到已保存的 Token，开始登录流程")
		if err := doLogin(cfg, wxClient, logger); err != nil {
			logger.Error("登录失败", "error", err)
			os.Exit(1)
		}
	}
	weixinConnected = true

	// 创建会话管理器
	sessionMgr := session.NewManager(&cfg.Session, logger.With("module", "session"))

	// 创建路由器并注册适配器
	msgRouter := router.NewRouter(&cfg.Routing, logger.With("module", "router"))
	registerAdapters(cfg, msgRouter, logger)

	// 创建消息发送器
	sender := weixin.NewSender(wxClient, logger.With("module", "sender"))

	// 创建管理后台服务
	store := server.NewStore(fmt.Sprintf("%s/runtime.json", cfg.Weixin.DataDir), logger.With("module", "store"))
	// 从当前配置初始化 store
	for _, a := range cfg.Adapters {
		store.AddAdapter(a)
	}
	store.SetRouting(cfg.Routing)

	adminServer := server.NewServer(store, sessionMgr, logger.With("module", "admin"))
	adminServer.MountFrontend(server.FrontendDist, "dist")

	// 设置状态回调
	accountID := loadAccountID(cfg)
	adminServer.SetStatusFunc(func() server.StatusInfo {
		return server.StatusInfo{
			WeixinConnected: weixinConnected,
			AccountID:       accountID,
			AdapterCount:    len(store.ListAdapters()),
			ActiveSessions:  sessionMgr.SessionCount(),
		}
	})

	// 启动 HTTP 管理服务器（后台）
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	go func() {
		if err := adminServer.ListenAndServe(addr); err != nil {
			logger.Error("管理服务器异常", "error", err)
		}
	}()

	// 设置 context，支持优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听系统信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("收到信号，正在关闭...", "signal", sig)
		cancel()
	}()

	// 启动消息轮询
	poller := weixin.NewPoller(wxClient,
		weixin.WithPollerAccountID(accountID),
		weixin.WithPollerLogger(logger.With("module", "poller")),
		weixin.WithPollerHandler(func(msg *weixin.WeixinMessage) {
			handleMessage(ctx, msg, accountID, wxClient, sender, msgRouter, sessionMgr, logger)
		}),
		weixin.WithBufUpdateCallback(func(buf string) {
			saveSyncBuf(cfg, accountID, buf, logger)
		}),
		weixin.WithInitialSyncBuf(loadSyncBuf(cfg, accountID, logger)),
	)

	logger.Info("🚀 WeClaw-Proxy 已启动",
		"accountID", accountID,
		"adapters", msgRouter.ListAdapters(),
		"adminPanel", "http://"+addr,
	)

	if err := poller.Start(ctx); err != nil && ctx.Err() == nil {
		logger.Error("轮询异常退出", "error", err)
		os.Exit(1)
	}

	logger.Info("WeClaw-Proxy 已停止")
}

// handleMessage 处理收到的微信消息
func handleMessage(
	ctx context.Context,
	msg *weixin.WeixinMessage,
	accountID string,
	wxClient *weixin.Client,
	sender *weixin.Sender,
	msgRouter *router.Router,
	sessionMgr *session.Manager,
	logger *slog.Logger,
) {
	fromUserID := msg.FromUserID
	if fromUserID == "" {
		return
	}

	// 提取文本内容
	text := weixin.ExtractTextFromMessage(msg)
	if text == "" {
		logger.Debug("跳过无文本内容的消息", "from", fromUserID)
		return
	}

	logger.Info("处理消息",
		"from", fromUserID,
		"text", truncate(text, 50),
	)

	// 更新 context_token
	if msg.ContextToken != "" {
		sessionMgr.UpdateContextToken(accountID, fromUserID, msg.ContextToken)
	}

	// 特殊命令处理
	if handleSpecialCommands(ctx, text, fromUserID, accountID, sender, sessionMgr, logger) {
		return
	}

	// 路由到适配器
	agentAdapter, cleanMsg, err := msgRouter.Route(fromUserID, text)
	if err != nil {
		logger.Error("路由失败", "error", err)
		replyText(ctx, sender, fromUserID, sessionMgr.GetContextToken(accountID, fromUserID),
			"⚠️ 未配置可用的 Agent，请检查配置文件。", logger)
		return
	}

	// 发送"正在输入"状态
	configResp, err := wxClient.GetConfig(ctx, fromUserID, sessionMgr.GetContextToken(accountID, fromUserID))
	if err == nil && configResp.TypingTicket != "" {
		_ = sender.SendTypingIndicator(ctx, fromUserID, configResp.TypingTicket, true)
	}

	// 构建 Agent 请求
	history := sessionMgr.GetHistory(accountID, fromUserID)
	chatReq := &adapter.ChatRequest{
		UserID:    fromUserID,
		Message:   cleanMsg,
		History:   history,
		SessionID: fmt.Sprintf("%s:%s", accountID, fromUserID),
	}

	// 记录用户消息到历史
	sessionMgr.AppendHistory(accountID, fromUserID, adapter.HistoryEntry{
		Role:    "user",
		Content: cleanMsg,
	})

	// 调用 Agent
	resp, err := agentAdapter.Chat(ctx, chatReq)
	if err != nil {
		logger.Error("Agent 调用失败",
			"adapter", agentAdapter.Name(),
			"error", err,
		)
		replyText(ctx, sender, fromUserID, sessionMgr.GetContextToken(accountID, fromUserID),
			fmt.Sprintf("⚠️ Agent 响应错误: %s", err.Error()), logger)
		return
	}

	// 取消"正在输入"状态
	if configResp != nil && configResp.TypingTicket != "" {
		_ = sender.SendTypingIndicator(ctx, fromUserID, configResp.TypingTicket, false)
	}

	// 发送回复
	if resp.Text != "" {
		replyText(ctx, sender, fromUserID, sessionMgr.GetContextToken(accountID, fromUserID),
			resp.Text, logger)

		// 记录助手回复到历史
		sessionMgr.AppendHistory(accountID, fromUserID, adapter.HistoryEntry{
			Role:    "assistant",
			Content: resp.Text,
		})
	}
}

// handleSpecialCommands 处理特殊命令
func handleSpecialCommands(
	ctx context.Context,
	text string,
	fromUserID string,
	accountID string,
	sender *weixin.Sender,
	sessionMgr *session.Manager,
	logger *slog.Logger,
) bool {
	switch strings.TrimSpace(text) {
	case "/clear", "/reset":
		sessionMgr.ClearHistory(accountID, fromUserID)
		replyText(ctx, sender, fromUserID, sessionMgr.GetContextToken(accountID, fromUserID),
			"✅ 对话历史已清除", logger)
		return true
	case "/help":
		helpText := `🤖 WeClaw-Proxy 帮助
/clear - 清除对话历史
/reset - 重置对话上下文
/help  - 显示帮助信息

直接发送消息即可与 Agent 对话。`
		replyText(ctx, sender, fromUserID, sessionMgr.GetContextToken(accountID, fromUserID),
			helpText, logger)
		return true
	}
	return false
}

// replyText 发送文本回复
func replyText(ctx context.Context, sender *weixin.Sender, to string, contextToken string, text string, logger *slog.Logger) {
	_, err := sender.SendText(ctx, to, text, contextToken)
	if err != nil {
		logger.Error("发送回复失败", "to", to, "error", err)
	}
}

// registerAdapters 根据配置注册适配器
func registerAdapters(cfg *config.Config, r *router.Router, logger *slog.Logger) {
	for _, acfg := range cfg.Adapters {
		acfgCopy := acfg // 避免闭包捕获循环变量
		var a adapter.Adapter
		adapterLogger := logger.With("adapter", acfg.Name)

		switch acfg.AdapterType {
		case "openai":
			a = adapter.NewOpenAIAdapter(&acfgCopy, adapterLogger)
		case "webhook":
			a = adapter.NewWebhookAdapter(&acfgCopy, adapterLogger)
		default:
			logger.Warn("暂不支持的适配器类型，跳过",
				"name", acfg.Name,
				"type", acfg.AdapterType,
			)
			continue
		}

		r.RegisterAdapter(a)
	}
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// --- 持久化辅助函数 ---

func loadSavedToken(cfg *config.Config, client *weixin.Client, logger *slog.Logger) bool {
	tokenFile := fmt.Sprintf("%s/token.json", cfg.Weixin.DataDir)
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return false
	}

	var accountData weixin.AccountData
	if err := json.Unmarshal(data, &accountData); err != nil {
		logger.Warn("解析 Token 文件失败", "error", err)
		return false
	}

	if accountData.Token == "" {
		return false
	}

	client.SetToken(accountData.Token)
	if accountData.BaseURL != "" {
		client.SetBaseURL(accountData.BaseURL)
	}

	logger.Info("已加载保存的 Token", "baseURL", accountData.BaseURL)
	return true
}

func doLogin(cfg *config.Config, client *weixin.Client, logger *slog.Logger) error {
	authClient := weixin.NewAuthClient(cfg.Weixin.BaseURL, logger.With("module", "auth"))

	result, err := authClient.Login(context.Background(),
		func(qr *weixin.QRCodeInfo) {
			fmt.Println("\n📱 请使用微信扫描以下二维码：")
			fmt.Printf("🔗 %s\n\n", qr.QRCodeURL)
		},
		func(msg string) {
			fmt.Println(msg)
		},
	)
	if err != nil {
		return err
	}

	if !result.Connected {
		return fmt.Errorf("登录失败: %s", result.Message)
	}

	// 保存 Token
	client.SetToken(result.BotToken)
	if result.BaseURL != "" {
		client.SetBaseURL(result.BaseURL)
	}

	// 持久化
	accountData := weixin.AccountData{
		Token:   result.BotToken,
		BaseURL: result.BaseURL,
		UserID:  result.UserID,
	}
	saveAccountData(cfg, weixin.NormalizeAccountID(result.AccountID), &accountData, logger)

	fmt.Println(result.Message)
	return nil
}

func saveAccountData(cfg *config.Config, accountID string, data *weixin.AccountData, logger *slog.Logger) {
	dir := cfg.Weixin.DataDir
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("创建数据目录失败", "error", err)
		return
	}

	tokenFile := fmt.Sprintf("%s/token.json", dir)
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		logger.Error("序列化 Token 数据失败", "error", err)
		return
	}
	if err := os.WriteFile(tokenFile, jsonData, 0600); err != nil {
		logger.Error("保存 Token 失败", "error", err)
	}

	accountFile := fmt.Sprintf("%s/account.txt", dir)
	_ = os.WriteFile(accountFile, []byte(accountID), 0600)
}

func loadAccountID(cfg *config.Config) string {
	accountFile := fmt.Sprintf("%s/account.txt", cfg.Weixin.DataDir)
	data, err := os.ReadFile(accountFile)
	if err != nil {
		return "default"
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "default"
	}
	return id
}

func saveSyncBuf(cfg *config.Config, accountID string, buf string, logger *slog.Logger) {
	dir := cfg.Weixin.DataDir
	_ = os.MkdirAll(dir, 0755)
	syncFile := fmt.Sprintf("%s/sync_%s.buf", dir, accountID)
	if err := os.WriteFile(syncFile, []byte(buf), 0600); err != nil {
		logger.Error("保存同步游标失败", "error", err)
	}
}

func loadSyncBuf(cfg *config.Config, accountID string, logger *slog.Logger) string {
	syncFile := fmt.Sprintf("%s/sync_%s.buf", cfg.Weixin.DataDir, accountID)
	data, err := os.ReadFile(syncFile)
	if err != nil {
		return ""
	}
	logger.Info("已加载同步游标", "size", len(data))
	return string(data)
}
