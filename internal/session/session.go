package session

import (
	"log/slog"
	"sync"
	"time"

	"github.com/amigoer/weclaw-proxy/internal/adapter"
)

// Session 用户会话
type Session struct {
	UserID       string                 // 微信用户 ID
	AccountID    string                 // 关联的 Bot 账号 ID
	ContextToken string                 // 微信 context_token（回复时必须回传）
	History      []adapter.HistoryEntry // 对话历史
	LastActive   time.Time              // 最后活跃时间
	Metadata     map[string]string      // 自定义元数据
}

// Manager 会话管理器
type Manager struct {
	sessions     map[string]*Session // key: accountID:userID
	mu           sync.RWMutex
	historyLimit int           // 历史记录最大条数
	timeout      time.Duration // 会话超时时间
	logger       *slog.Logger
}

// ManagerConfig 会话管理器配置
type ManagerConfig struct {
	HistoryLimit   int `yaml:"history_limit"`   // 历史记录最大条数
	TimeoutMinutes int `yaml:"timeout_minutes"` // 超时时间（分钟）
}

// NewManager 创建新的会话管理器
func NewManager(cfg *ManagerConfig, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}

	historyLimit := 20
	timeout := 30 * time.Minute

	if cfg != nil {
		if cfg.HistoryLimit > 0 {
			historyLimit = cfg.HistoryLimit
		}
		if cfg.TimeoutMinutes > 0 {
			timeout = time.Duration(cfg.TimeoutMinutes) * time.Minute
		}
	}

	m := &Manager{
		sessions:     make(map[string]*Session),
		historyLimit: historyLimit,
		timeout:      timeout,
		logger:       logger,
	}

	// 启动定期清理
	go m.cleanupLoop()

	return m
}

// sessionKey 生成会话 key
func sessionKey(accountID, userID string) string {
	return accountID + ":" + userID
}

// GetOrCreate 获取或创建会话
func (m *Manager) GetOrCreate(accountID, userID string) *Session {
	key := sessionKey(accountID, userID)

	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[key]
	if ok && time.Since(s.LastActive) < m.timeout {
		s.LastActive = time.Now()
		return s
	}

	// 创建新会话（或会话已超时）
	if ok {
		m.logger.Info("会话已超时，重新创建",
			"accountID", accountID,
			"userID", userID,
		)
	}

	s = &Session{
		UserID:     userID,
		AccountID:  accountID,
		History:    make([]adapter.HistoryEntry, 0),
		LastActive: time.Now(),
		Metadata:   make(map[string]string),
	}
	m.sessions[key] = s
	return s
}

// UpdateContextToken 更新会话的 context_token
func (m *Manager) UpdateContextToken(accountID, userID, token string) {
	key := sessionKey(accountID, userID)

	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[key]
	if !ok {
		s = &Session{
			UserID:     userID,
			AccountID:  accountID,
			History:    make([]adapter.HistoryEntry, 0),
			LastActive: time.Now(),
			Metadata:   make(map[string]string),
		}
		m.sessions[key] = s
	}
	s.ContextToken = token
	m.logger.Debug("context_token 已更新",
		"accountID", accountID,
		"userID", userID,
	)
}

// GetContextToken 获取会话的 context_token
func (m *Manager) GetContextToken(accountID, userID string) string {
	key := sessionKey(accountID, userID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[key]
	if !ok {
		return ""
	}
	return s.ContextToken
}

// AppendHistory 追加对话历史
func (m *Manager) AppendHistory(accountID, userID string, entry adapter.HistoryEntry) {
	key := sessionKey(accountID, userID)

	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[key]
	if !ok {
		return
	}

	s.History = append(s.History, entry)
	s.LastActive = time.Now()

	// 超出限制时截断
	if len(s.History) > m.historyLimit {
		s.History = s.History[len(s.History)-m.historyLimit:]
	}
}

// GetHistory 获取对话历史
func (m *Manager) GetHistory(accountID, userID string) []adapter.HistoryEntry {
	key := sessionKey(accountID, userID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[key]
	if !ok {
		return nil
	}

	// 返回副本
	history := make([]adapter.HistoryEntry, len(s.History))
	copy(history, s.History)
	return history
}

// ClearHistory 清除对话历史
func (m *Manager) ClearHistory(accountID, userID string) {
	key := sessionKey(accountID, userID)

	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[key]
	if ok {
		s.History = make([]adapter.HistoryEntry, 0)
		m.logger.Info("对话历史已清除",
			"accountID", accountID,
			"userID", userID,
		)
	}
}

// cleanupLoop 定期清理过期会话
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for key, s := range m.sessions {
			if now.Sub(s.LastActive) > m.timeout {
				delete(m.sessions, key)
				m.logger.Debug("过期会话已清理", "key", key)
			}
		}
		m.mu.Unlock()
	}
}

// SessionCount 获取活跃会话数量
func (m *Manager) SessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
