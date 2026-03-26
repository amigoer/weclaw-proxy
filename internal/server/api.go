package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/amigoer/weclaw-proxy/internal/adapter"
	"github.com/amigoer/weclaw-proxy/internal/router"
	"github.com/amigoer/weclaw-proxy/internal/session"
	"github.com/amigoer/weclaw-proxy/internal/weixin"
)

// AccountInfo 单个微信账号信息
type AccountInfo struct {
	AccountID string `json:"account_id"`
	Nickname  string `json:"nickname,omitempty"`
	Connected bool   `json:"connected"`
}

// StatusInfo 系统状态信息
type StatusInfo struct {
	WeixinConnected     bool          `json:"weixin_connected"`
	AccountID           string        `json:"account_id"`
	Accounts            []AccountInfo `json:"accounts,omitempty"`
	AdapterCount        int           `json:"adapter_count"`
	ActiveSessions      int           `json:"active_sessions"`
	SmartRoutingEnabled bool          `json:"smart_routing_enabled"`
	Uptime              string        `json:"uptime"`
}

// loginState Web 登录状态
type loginState struct {
	mu        sync.Mutex
	active    bool   // 是否有正在进行的登录
	status    string // wait / scaned / confirmed / expired / error
	message   string // 状态消息
	accountID string // confirmed 后保存的账号 ID
	qrURL     string // 二维码 URL
	qrCode    string // 二维码值（用于轮询）
	cancel    context.CancelFunc
}

// LoginSuccessCallback 登录成功回调
type LoginSuccessCallback func(result *weixin.LoginResult) error

// Server Web 管理服务器
type Server struct {
	store           *Store
	sessionMgr      *session.Manager
	statusFn        func() StatusInfo
	logoutFn        func() error                     // 退出所有账号（向后兼容）
	logoutAccountFn func(accountID string) error      // 退出指定账号
	renameAccountFn func(accountID, nickname string) error // 修改账号备注
	loginCb         LoginSuccessCallback              // 登录成功回调
	authClient      *weixin.AuthClient                // 认证客户端
	login           loginState                        // 登录状态
	logger          *slog.Logger
	mux             *http.ServeMux
}

// NewServer 创建管理服务器
func NewServer(store *Store, sessionMgr *session.Manager, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		store:      store,
		sessionMgr: sessionMgr,
		logger:     logger,
		mux:        http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// SetStatusFunc 设置状态获取函数
func (s *Server) SetStatusFunc(fn func() StatusInfo) {
	s.statusFn = fn
}

// SetLogoutFunc 设置退出登录回调
func (s *Server) SetLogoutFunc(fn func() error) {
	s.logoutFn = fn
}

// SetLogoutAccountFunc 设置按账号退出回调
func (s *Server) SetLogoutAccountFunc(fn func(accountID string) error) {
	s.logoutAccountFn = fn
}

// SetRenameAccountFunc 设置修改账号备注回调
func (s *Server) SetRenameAccountFunc(fn func(accountID, nickname string) error) {
	s.renameAccountFn = fn
}

// SetLoginCallback 设置登录成功回调
func (s *Server) SetLoginCallback(cb LoginSuccessCallback) {
	s.loginCb = cb
}

// SetAuthClient 设置认证客户端
func (s *Server) SetAuthClient(ac *weixin.AuthClient) {
	s.authClient = ac
}


// Handler 返回 HTTP Handler
func (s *Server) Handler() http.Handler {
	return s.mux
}

// registerRoutes 注册 API 路由
func (s *Server) registerRoutes() {
	// API 路由
	s.mux.HandleFunc("/api/status", s.cors(s.handleStatus))
	s.mux.HandleFunc("/api/adapters", s.cors(s.handleAdapters))
	s.mux.HandleFunc("/api/adapters/models", s.cors(s.handleModels))
	s.mux.HandleFunc("/api/adapters/", s.cors(s.handleAdapterByName))
	s.mux.HandleFunc("/api/routes", s.cors(s.handleRoutes))
	s.mux.HandleFunc("/api/smart-routing", s.cors(s.handleSmartRouting))
	s.mux.HandleFunc("/api/logout", s.cors(s.handleLogout))
	s.mux.HandleFunc("/api/accounts/", s.cors(s.handleAccountByID))
	s.mux.HandleFunc("/api/login/qrcode", s.cors(s.handleLoginQRCode))
	s.mux.HandleFunc("/api/login/status", s.cors(s.handleLoginStatus))

	// 前端静态文件（由 main.go 挂载）
}

// MountFrontend 挂载前端静态文件
func (s *Server) MountFrontend(efs embed.FS, subDir string) {
	distFS, err := fs.Sub(efs, subDir)
	if err != nil {
		s.logger.Error("加载前端资源失败", "error", err)
		return
	}
	fileServer := http.FileServer(http.FS(distFS))

	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if _, err := fs.Stat(distFS, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// cors CORS 中间件
func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// --- API Handlers ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := StatusInfo{
		AdapterCount:   len(s.store.ListAdapters()),
		ActiveSessions: s.sessionMgr.SessionCount(),
	}
	if s.statusFn != nil {
		status = s.statusFn()
	}
	s.json(w, status)
}

func (s *Server) handleAdapters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		adapters := s.store.ListAdapters()
		for i := range adapters {
			if adapters[i].APIKey != "" {
				adapters[i].APIKey = maskKey(adapters[i].APIKey)
			}
		}
		s.json(w, adapters)

	case http.MethodPost:
		var cfg adapter.AdapterConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			s.jsonErr(w, "无效的请求体", http.StatusBadRequest)
			return
		}
		if cfg.Name == "" {
			s.jsonErr(w, "名称不能为空", http.StatusBadRequest)
			return
		}
		if s.store.GetAdapter(cfg.Name) != nil {
			s.jsonErr(w, "名称已存在", http.StatusConflict)
			return
		}
		s.store.AddAdapter(cfg)
		_ = s.store.Save()
		_ = s.store.SaveToYAML()
		s.logger.Info("适配器已添加", "name", cfg.Name)
		s.json(w, map[string]string{"status": "ok", "name": cfg.Name})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdapterByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/adapters/")
	if name == "" {
		s.jsonErr(w, "名称不能为空", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg := s.store.GetAdapter(name)
		if cfg == nil {
			s.jsonErr(w, "不存在", http.StatusNotFound)
			return
		}
		masked := *cfg
		if masked.APIKey != "" {
			masked.APIKey = maskKey(masked.APIKey)
		}
		s.json(w, masked)

	case http.MethodPut:
		var cfg adapter.AdapterConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			s.jsonErr(w, "无效的请求体", http.StatusBadRequest)
			return
		}
		cfg.Name = name
		if isMasked(cfg.APIKey) {
			if existing := s.store.GetAdapter(name); existing != nil {
				cfg.APIKey = existing.APIKey
			}
		}
		if !s.store.UpdateAdapter(name, cfg) {
			s.jsonErr(w, "不存在", http.StatusNotFound)
			return
		}
		_ = s.store.Save()
		_ = s.store.SaveToYAML()
		s.json(w, map[string]string{"status": "ok"})

	case http.MethodDelete:
		if !s.store.DeleteAdapter(name) {
			s.jsonErr(w, "不存在", http.StatusNotFound)
			return
		}
		// 级联清理：如果删除的是默认适配器，自动清除引用
		routing := s.store.GetRouting()
		if routing.DefaultAdapter == name {
			routing.DefaultAdapter = ""
			// 自动回落到第一个可用适配器
			if adapters := s.store.ListAdapters(); len(adapters) > 0 {
				routing.DefaultAdapter = adapters[0].Name
			}
			s.store.SetRouting(routing)
			s.logger.Info("默认适配器已自动调整", "newDefault", routing.DefaultAdapter)
		}
		_ = s.store.Save()
		_ = s.store.SaveToYAML()
		s.json(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// routeRulePayload API 路由规则载荷
type routeRulePayload struct {
	Match struct {
		Prefix  string   `json:"prefix,omitempty"`
		UserIDs []string `json:"user_ids,omitempty"`
	} `json:"match"`
	Adapter string `json:"adapter"`
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		routing := s.store.GetRouting()
		s.json(w, routing)

	case http.MethodPut:
		var payload struct {
			DefaultAdapter string             `json:"default_adapter"`
			Rules          []routeRulePayload `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.jsonErr(w, "无效的请求体", http.StatusBadRequest)
			return
		}
		rules := make([]router.RouteRule, len(payload.Rules))
		for i, rp := range payload.Rules {
			rules[i] = router.RouteRule{
				Match: router.MatchRule{
					Prefix:  rp.Match.Prefix,
					UserIDs: rp.Match.UserIDs,
				},
				AdapterName: rp.Adapter,
			}
		}
		s.store.SetRouting(router.RouterConfig{
			DefaultAdapter: payload.DefaultAdapter,
			Rules:          rules,
		})
		_ = s.store.Save()
		_ = s.store.SaveToYAML()
		s.json(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSmartRouting(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := s.store.GetSmartRouting()
		// 脱敏 API Key
		masked := cfg
		if masked.APIKey != "" {
			masked.APIKey = maskKey(masked.APIKey)
		}
		s.json(w, masked)

	case http.MethodPut:
		var cfg router.SmartRoutingConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			s.jsonErr(w, "无效的请求体", http.StatusBadRequest)
			return
		}
		// 保留原 API Key（如果前端发回的是脱敏值）
		if isMasked(cfg.APIKey) {
			existing := s.store.GetSmartRouting()
			cfg.APIKey = existing.APIKey
		}
		s.store.SetSmartRouting(cfg)
		_ = s.store.Save()
		_ = s.store.SaveToYAML()
		s.json(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- 辅助 ---

func (s *Server) json(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func isMasked(key string) bool {
	return strings.Contains(key, "****")
}

// handleLoginQRCode 获取登录二维码并开始后台登录流程
func (s *Server) handleLoginQRCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authClient == nil {
		s.jsonErr(w, "认证客户端未配置", http.StatusInternalServerError)
		return
	}

	s.login.mu.Lock()
	// 取消之前的登录流程
	if s.login.cancel != nil {
		s.login.cancel()
	}
	s.login.active = true
	s.login.status = "wait"
	s.login.message = "正在获取二维码..."
	s.login.mu.Unlock()

	// 获取二维码
	qrInfo, err := s.authClient.FetchQRCode(r.Context(), weixin.DefaultBotType)
	if err != nil {
		s.login.mu.Lock()
		s.login.status = "error"
		s.login.message = "获取二维码失败: " + err.Error()
		s.login.active = false
		s.login.mu.Unlock()
		s.jsonErr(w, "获取二维码失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.login.mu.Lock()
	s.login.qrURL = qrInfo.QRCodeURL
	s.login.qrCode = qrInfo.QRCode
	s.login.status = "wait"
	s.login.message = "请扫描二维码"
	s.login.mu.Unlock()

	// 后台轮询登录状态
	ctx, cancel := context.WithCancel(context.Background())
	s.login.mu.Lock()
	s.login.cancel = cancel
	s.login.mu.Unlock()

	go s.pollLoginStatus(ctx, qrInfo.QRCode)

	s.json(w, map[string]string{
		"status": "ok",
		"qr_url": qrInfo.QRCodeURL,
	})
}

// pollLoginStatus 后台轮询二维码状态
func (s *Server) pollLoginStatus(ctx context.Context, qrCode string) {
	defer func() {
		s.login.mu.Lock()
		s.login.active = false
		s.login.cancel = nil
		s.login.mu.Unlock()
	}()

	currentQR := qrCode
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		statusResp, err := s.authClient.PollQRStatus(ctx, currentQR)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.login.mu.Lock()
			s.login.status = "error"
			s.login.message = "轮询失败: " + err.Error()
			s.login.mu.Unlock()
			return
		}

		s.login.mu.Lock()
		switch statusResp.Status {
		case "wait":
			s.login.status = "wait"
		case "scaned":
			s.login.status = "scaned"
			s.login.message = "已扫码，请在微信中确认"
		case "expired":
			// 尝试刷新二维码
			s.login.status = "expired"
			s.login.message = "二维码已过期，正在刷新..."
			s.login.mu.Unlock()

			newQR, err := s.authClient.FetchQRCode(ctx, weixin.DefaultBotType)
			if err != nil {
				s.login.mu.Lock()
				s.login.status = "error"
				s.login.message = "刷新二维码失败"
				s.login.mu.Unlock()
				return
			}
			s.login.mu.Lock()
			s.login.qrURL = newQR.QRCodeURL
			s.login.qrCode = newQR.QRCode
			currentQR = newQR.QRCode
			s.login.status = "wait"
			s.login.message = "二维码已刷新，请重新扫码"
			s.login.mu.Unlock()
			continue

		case "confirmed":
			s.login.status = "confirmed"
			s.login.message = "登录成功！"
			s.login.accountID = weixin.NormalizeAccountID(statusResp.ILinkBotID)
			s.login.mu.Unlock()

			// 调用登录成功回调
			if s.loginCb != nil {
				baseURL := statusResp.BaseURL
				if baseURL == "" {
					baseURL = s.authClient.GetBaseURL()
				}
				result := &weixin.LoginResult{
					Connected: true,
					BotToken:  statusResp.BotToken,
					AccountID: statusResp.ILinkBotID,
					BaseURL:   baseURL,
					UserID:    statusResp.ILinkUserID,
					Message:   "登录成功",
				}
				if err := s.loginCb(result); err != nil {
					s.logger.Error("登录回调失败", "error", err)
				}
			}
			return
		}
		s.login.mu.Unlock()
	}
}

// handleLoginStatus 获取登录状态
func (s *Server) handleLoginStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.login.mu.Lock()
	defer s.login.mu.Unlock()

	s.json(w, map[string]interface{}{
		"active":     s.login.active,
		"status":     s.login.status,
		"message":    s.login.message,
		"qr_url":     s.login.qrURL,
		"account_id": s.login.accountID,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.logoutFn == nil {
		s.jsonErr(w, "退出功能未配置", http.StatusInternalServerError)
		return
	}
	if err := s.logoutFn(); err != nil {
		s.jsonErr(w, "退出失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.logger.Info("已退出微信登录")
	s.json(w, map[string]string{"status": "ok", "message": "已退出登录，请重新扫码"})
}

// handleAccountByID 按 accountID 操作账号
func (s *Server) handleAccountByID(w http.ResponseWriter, r *http.Request) {
	accountID := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
	if accountID == "" {
		s.jsonErr(w, "缺少 account_id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		// 修改账号备注
		var body struct {
			Nickname string `json:"nickname"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			s.jsonErr(w, "无效的请求体", http.StatusBadRequest)
			return
		}
		if s.renameAccountFn == nil {
			s.jsonErr(w, "备注功能未配置", http.StatusInternalServerError)
			return
		}
		if err := s.renameAccountFn(accountID, body.Nickname); err != nil {
			s.jsonErr(w, "修改备注失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.json(w, map[string]string{"status": "ok", "nickname": body.Nickname})

	case http.MethodDelete:
		if s.logoutAccountFn == nil {
			s.jsonErr(w, "退出功能未配置", http.StatusInternalServerError)
			return
		}
		if err := s.logoutAccountFn(accountID); err != nil {
			s.jsonErr(w, "退出失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.logger.Info("已退出指定微信账号", "accountID", accountID)
		s.json(w, map[string]string{"status": "ok", "account_id": accountID})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ListenAndServe 启动服务器
func (s *Server) ListenAndServe(addr string) error {
	s.logger.Info("管理后台已启动", "addr", addr)
	displayAddr := addr
	if strings.HasPrefix(displayAddr, ":") {
		displayAddr = "localhost" + displayAddr
	}
	fmt.Printf("🌐 管理面板: http://%s\n", displayAddr)
	return http.ListenAndServe(addr, s.mux)
}

// handleModels 代理查询远端模型列表
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	baseURL := r.URL.Query().Get("base_url")
	apiKey := r.URL.Query().Get("api_key")

	if baseURL == "" {
		s.jsonErr(w, "缺少 base_url 参数", http.StatusBadRequest)
		return
	}

	baseURL = strings.TrimRight(baseURL, "/")

	// 构建远端请求 URL
	modelsURL := baseURL + "/models"

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		s.jsonErr(w, "创建请求失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.jsonErr(w, "请求模型列表失败: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.jsonErr(w, "读取响应失败: "+err.Error(), http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		s.jsonErr(w, fmt.Sprintf("远端返回 HTTP %d", resp.StatusCode), resp.StatusCode)
		return
	}

	// 解析 OpenAI 格式的模型列表
	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		// 非 OpenAI 标准格式，尝试返回原始响应
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
		return
	}

	// 提取模型 ID 列表
	models := make([]string, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}

	s.json(w, map[string]interface{}{"models": models})
}
