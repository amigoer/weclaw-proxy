package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	qrterminal "github.com/mdp/qrterminal/v3"

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

// accountSession 单个微信账号的运行时状态
type accountSession struct {
	accountID  string
	nickname   string
	client     *weixin.Client
	sender     *weixin.Sender
	sessionMgr *session.Manager
	cancel     context.CancelFunc
}

// accountManager 多账号管理器
type accountManager struct {
	mu       sync.RWMutex
	sessions map[string]*accountSession
}

func newAccountManager() *accountManager {
	return &accountManager{
		sessions: make(map[string]*accountSession),
	}
}

func (m *accountManager) add(s *accountSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.accountID] = s
}

func (m *accountManager) remove(accountID string) *accountSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[accountID]
	if ok {
		delete(m.sessions, accountID)
	}
	return s
}

func (m *accountManager) get(accountID string) *accountSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[accountID]
}

func (m *accountManager) list() []*accountSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*accountSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

func (m *accountManager) count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

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

	// 仅登录模式
	if *loginOnly {
		wxClient := weixin.NewClient(
			weixin.WithBaseURL(cfg.Weixin.BaseURL),
			weixin.WithCDNBaseURL(cfg.Weixin.CDNBaseURL),
			weixin.WithLongPollTimeout(cfg.Weixin.LongPollTimeoutMs),
			weixin.WithLogger(logger.With("module", "weixin")),
		)
		if err := doLogin(cfg, wxClient, logger); err != nil {
			logger.Error("登录失败", "error", err)
			os.Exit(1)
		}
		return
	}

	// 创建多账号管理器
	mgr := newAccountManager()

	// 创建路由器并注册适配器（所有账号共享）
	msgRouter := router.NewRouter(&cfg.Routing, logger.With("module", "router"))
	registerAdapters(cfg, msgRouter, logger)

	// 初始化智能路由（如果启用）
	if cfg.SmartRouting.Enabled {
		smartRouter := router.NewSmartRouter(&cfg.SmartRouting, logger.With("module", "smart-router"))
		msgRouter.SetSmartRouter(smartRouter)
	}

	// 创建管理后台服务
	store := server.NewStore(fmt.Sprintf("%s/runtime.json", cfg.Weixin.DataDir), logger.With("module", "store"))
	store.SetConfigFilePath(*configPath)
	for _, a := range cfg.Adapters {
		store.AddAdapter(a)
	}
	store.SetRouting(cfg.Routing)
	store.SetSmartRouting(cfg.SmartRouting)

	// 创建共享会话管理器（用于 StatusInfo 统计）
	globalSessionMgr := session.NewManager(&cfg.Session, logger.With("module", "session"))
	adminServer := server.NewServer(store, globalSessionMgr, logger.With("module", "admin"))
	adminServer.MountFrontend(server.FrontendDist, "dist")

	// 设置认证客户端（Web 登录用）
	authClient := weixin.NewAuthClient(cfg.Weixin.BaseURL, logger.With("module", "auth"))
	adminServer.SetAuthClient(authClient)

	// 设置 context，支持优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置 Web 登录成功回调
	adminServer.SetLoginCallback(func(result *weixin.LoginResult) error {
		accountID := weixin.NormalizeAccountID(result.AccountID)

		// 创建新账号的微信客户端
		newClient := weixin.NewClient(
			weixin.WithBaseURL(cfg.Weixin.BaseURL),
			weixin.WithCDNBaseURL(cfg.Weixin.CDNBaseURL),
			weixin.WithLongPollTimeout(cfg.Weixin.LongPollTimeoutMs),
			weixin.WithLogger(logger.With("module", "weixin", "account", accountID)),
		)
		newClient.SetToken(result.BotToken)
		if result.BaseURL != "" {
			newClient.SetBaseURL(result.BaseURL)
		}

		newSender := weixin.NewSender(newClient, logger.With("module", "sender", "account", accountID))
		newSessionMgr := session.NewManager(&cfg.Session, logger.With("module", "session", "account", accountID))

		sessCtx, sessCancel := context.WithCancel(ctx)
		sess := &accountSession{
			accountID:  accountID,
			client:     newClient,
			sender:     newSender,
			sessionMgr: newSessionMgr,
			cancel:     sessCancel,
		}
		mgr.add(sess)

		// 持久化 Token
		accountData := weixin.AccountData{
			Token:   result.BotToken,
			BaseURL: result.BaseURL,
			UserID:  result.UserID,
		}
		saveAccountData(cfg, accountID, &accountData, logger)
		logger.Info("Web 登录成功", "accountID", accountID, "totalAccounts", mgr.count())

		// 启动消息轮询
		go startPoller(sessCtx, cfg, newClient, newSender, msgRouter, newSessionMgr, accountID, logger)
		return nil
	})

	// 设置状态回调
	adminServer.SetStatusFunc(func() server.StatusInfo {
		accounts := mgr.list()
		accountInfos := make([]server.AccountInfo, 0, len(accounts))
		for _, s := range accounts {
			accountInfos = append(accountInfos, server.AccountInfo{
				AccountID: s.accountID,
				Nickname:  s.nickname,
				Connected: true,
			})
		}

		// 向后兼容：单账号时填充老字段
		connected := len(accounts) > 0
		firstID := ""
		if connected {
			firstID = accounts[0].accountID
		}

		return server.StatusInfo{
			WeixinConnected:     connected,
			AccountID:           firstID,
			Accounts:            accountInfos,
			AdapterCount:        len(store.ListAdapters()),
			ActiveSessions:      globalSessionMgr.SessionCount(),
			SmartRoutingEnabled: msgRouter.SmartRouterEnabled(),
		}
	})

	// 设置退出登录回调（退出所有账号）
	adminServer.SetLogoutFunc(func() error {
		for _, s := range mgr.list() {
			if s.cancel != nil {
				s.cancel()
			}
			removeAccountData(cfg, s.accountID, logger)
			mgr.remove(s.accountID)
		}
		logger.Info("已退出所有微信账号")
		return nil
	})

	// 设置按账号退出回调
	adminServer.SetLogoutAccountFunc(func(accountID string) error {
		s := mgr.remove(accountID)
		if s == nil {
			return fmt.Errorf("账号不存在: %s", accountID)
		}
		if s.cancel != nil {
			s.cancel()
		}
		removeAccountData(cfg, accountID, logger)
		logger.Info("已退出微信账号", "accountID", accountID, "remaining", mgr.count())
		return nil
	})

	// 设置修改账号备注回调
	adminServer.SetRenameAccountFunc(func(accountID, nickname string) error {
		s := mgr.get(accountID)
		if s == nil {
			return fmt.Errorf("账号不存在: %s", accountID)
		}
		s.nickname = nickname
		// 持久化备注
		accountData := loadAccountData(cfg, accountID)
		if accountData != nil {
			accountData.Nickname = nickname
			saveAccountData(cfg, accountID, accountData, logger)
		}
		logger.Info("已修改账号备注", "accountID", accountID, "nickname", nickname)
		return nil
	})

	// 启动 HTTP 管理服务器（后台）
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	go func() {
		if err := adminServer.ListenAndServe(addr); err != nil {
			logger.Error("管理服务器异常", "error", err)
		}
	}()

	// 监听系统信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("收到信号，正在关闭...", "signal", sig)
		cancel()
	}()

	// 尝试从持久化数据恢复已登录账号
	restoredAccounts := restoreAccounts(ctx, cfg, mgr, msgRouter, logger)

	if restoredAccounts > 0 {
		logger.Info("🚀 WeClaw-Proxy 已启动",
			"accounts", restoredAccounts,
			"adapters", msgRouter.ListAdapters(),
			"adminPanel", "http://"+addr,
		)
	} else {
		displayAddr := addr
		if strings.HasPrefix(displayAddr, ":") {
			displayAddr = "localhost" + displayAddr
		}
		logger.Info("⏳ 等待登录，CLI 扫码或访问管理面板均可",
			"adminPanel", "http://"+displayAddr,
		)
		fmt.Printf("\n🌐 访问 http://%s 在 Web 管理面板扫码登录\n\n", displayAddr)
	}

	// 等待信号退出
	<-ctx.Done()
	logger.Info("WeClaw-Proxy 已停止")
}

// startPoller 启动消息轮询
func startPoller(
	ctx context.Context,
	cfg *config.Config,
	wxClient *weixin.Client,
	sender *weixin.Sender,
	msgRouter *router.Router,
	sessionMgr *session.Manager,
	accountID string,
	logger *slog.Logger,
) {
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

	logger.Info("消息轮询启动中", "accountID", accountID)

	if err := poller.Start(ctx); err != nil && ctx.Err() == nil {
		logger.Error("轮询异常退出", "error", err)
	}
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
	agentAdapter, cleanMsg, err := msgRouter.RouteWithContext(ctx, fromUserID, text)
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
		case "cli":
			a = adapter.NewCLIAdapter(&acfgCopy, adapterLogger)
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
			qrterminal.GenerateHalfBlock(qr.QRCodeURL, qrterminal.L, os.Stdout)
			fmt.Printf("\n🔗 %s\n\n", qr.QRCodeURL)
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

func loadAccountData(cfg *config.Config, accountID string) *weixin.AccountData {
	tokenFile := filepath.Join(cfg.Weixin.DataDir, "accounts", accountID, "token.json")
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil
	}
	var accountData weixin.AccountData
	if err := json.Unmarshal(data, &accountData); err != nil {
		return nil
	}
	return &accountData
}

func saveAccountData(cfg *config.Config, accountID string, data *weixin.AccountData, logger *slog.Logger) {
	dir := filepath.Join(cfg.Weixin.DataDir, "accounts", accountID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("创建账号数据目录失败", "error", err)
		return
	}

	tokenFile := filepath.Join(dir, "token.json")
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		logger.Error("序列化 Token 数据失败", "error", err)
		return
	}
	if err := os.WriteFile(tokenFile, jsonData, 0600); err != nil {
		logger.Error("保存 Token 失败", "error", err)
	}
}

func removeAccountData(cfg *config.Config, accountID string, logger *slog.Logger) {
	dir := filepath.Join(cfg.Weixin.DataDir, "accounts", accountID)
	if err := os.RemoveAll(dir); err != nil {
		logger.Error("删除账号数据失败", "accountID", accountID, "error", err)
	}
	// 兼容旧版单账号文件
	_ = os.Remove(filepath.Join(cfg.Weixin.DataDir, "token.json"))
	_ = os.Remove(filepath.Join(cfg.Weixin.DataDir, "account.txt"))
}

// restoreAccounts 从持久化数据恢复所有已登录账号
func restoreAccounts(
	ctx context.Context,
	cfg *config.Config,
	mgr *accountManager,
	msgRouter *router.Router,
	logger *slog.Logger,
) int {
	restored := 0

	// 扫描 accounts 子目录
	accountsDir := filepath.Join(cfg.Weixin.DataDir, "accounts")
	entries, err := os.ReadDir(accountsDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			accountID := entry.Name()
			if restoreOneAccount(ctx, cfg, mgr, msgRouter, accountID, logger) {
				restored++
			}
		}
	}

	// 兼容旧版单账号 token.json
	if restored == 0 {
		oldTokenFile := filepath.Join(cfg.Weixin.DataDir, "token.json")
		if data, err := os.ReadFile(oldTokenFile); err == nil {
			var accountData weixin.AccountData
			if json.Unmarshal(data, &accountData) == nil && accountData.Token != "" {
				accountID := loadAccountID(cfg)
				client := weixin.NewClient(
					weixin.WithBaseURL(cfg.Weixin.BaseURL),
					weixin.WithCDNBaseURL(cfg.Weixin.CDNBaseURL),
					weixin.WithLongPollTimeout(cfg.Weixin.LongPollTimeoutMs),
					weixin.WithLogger(logger.With("module", "weixin", "account", accountID)),
				)
				client.SetToken(accountData.Token)
				if accountData.BaseURL != "" {
					client.SetBaseURL(accountData.BaseURL)
				}

				sender := weixin.NewSender(client, logger.With("module", "sender", "account", accountID))
				sessionMgr := session.NewManager(&cfg.Session, logger.With("module", "session", "account", accountID))

				sessCtx, sessCancel := context.WithCancel(ctx)
				sess := &accountSession{
					accountID:  accountID,
					client:     client,
					sender:     sender,
					sessionMgr: sessionMgr,
					cancel:     sessCancel,
				}
				mgr.add(sess)

				// 迁移到新目录结构
				saveAccountData(cfg, accountID, &accountData, logger)

				go startPoller(sessCtx, cfg, client, sender, msgRouter, sessionMgr, accountID, logger)
				logger.Info("已恢复账号（旧版迁移）", "accountID", accountID)
				restored++
			}
		}
	}

	return restored
}

// restoreOneAccount 恢复单个账号
func restoreOneAccount(
	ctx context.Context,
	cfg *config.Config,
	mgr *accountManager,
	msgRouter *router.Router,
	accountID string,
	logger *slog.Logger,
) bool {
	tokenFile := filepath.Join(cfg.Weixin.DataDir, "accounts", accountID, "token.json")
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return false
	}

	var accountData weixin.AccountData
	if err := json.Unmarshal(data, &accountData); err != nil || accountData.Token == "" {
		return false
	}

	client := weixin.NewClient(
		weixin.WithBaseURL(cfg.Weixin.BaseURL),
		weixin.WithCDNBaseURL(cfg.Weixin.CDNBaseURL),
		weixin.WithLongPollTimeout(cfg.Weixin.LongPollTimeoutMs),
		weixin.WithLogger(logger.With("module", "weixin", "account", accountID)),
	)
	client.SetToken(accountData.Token)
	if accountData.BaseURL != "" {
		client.SetBaseURL(accountData.BaseURL)
	}

	sender := weixin.NewSender(client, logger.With("module", "sender", "account", accountID))
	sessionMgr := session.NewManager(&cfg.Session, logger.With("module", "session", "account", accountID))

	sessCtx, sessCancel := context.WithCancel(ctx)
	sess := &accountSession{
		accountID:  accountID,
		nickname:   accountData.Nickname,
		client:     client,
		sender:     sender,
		sessionMgr: sessionMgr,
		cancel:     sessCancel,
	}
	mgr.add(sess)

	go startPoller(sessCtx, cfg, client, sender, msgRouter, sessionMgr, accountID, logger)
	logger.Info("已恢复账号", "accountID", accountID)
	return true
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
