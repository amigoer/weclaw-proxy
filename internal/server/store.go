package server

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"

	"github.com/amigoer/weclaw-proxy/internal/adapter"
	"github.com/amigoer/weclaw-proxy/internal/router"
)

// RuntimeConfig 运行时配置（支持热更新）
type RuntimeConfig struct {
	Adapters       []adapter.AdapterConfig `json:"adapters"`
	Routing        router.RouterConfig     `json:"routing"`
	HistoryLimit   int                     `json:"history_limit"`
	TimeoutMinutes int                    `json:"timeout_minutes"`
}

// Store 运行时配置存储
type Store struct {
	config   RuntimeConfig
	filePath string
	mu       sync.RWMutex
	logger   *slog.Logger

	// 配置变更回调
	onUpdate func(cfg *RuntimeConfig)
}

// NewStore 创建配置存储
func NewStore(filePath string, logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{
		filePath: filePath,
		logger:   logger,
		config: RuntimeConfig{
			Adapters:       make([]adapter.AdapterConfig, 0),
			HistoryLimit:   20,
			TimeoutMinutes: 30,
		},
	}
}

// SetOnUpdate 设置配置变更回调
func (s *Store) SetOnUpdate(fn func(cfg *RuntimeConfig)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onUpdate = fn
}

// Load 从文件加载配置
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在则使用默认配置
		}
		return err
	}

	return json.Unmarshal(data, &s.config)
}

// Save 保存配置到文件
func (s *Store) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.config, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// GetConfig 获取当前配置副本
func (s *Store) GetConfig() RuntimeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 深拷贝 adapters
	adapters := make([]adapter.AdapterConfig, len(s.config.Adapters))
	copy(adapters, s.config.Adapters)

	rules := make([]router.RouteRule, len(s.config.Routing.Rules))
	copy(rules, s.config.Routing.Rules)

	return RuntimeConfig{
		Adapters: adapters,
		Routing: router.RouterConfig{
			DefaultAdapter: s.config.Routing.DefaultAdapter,
			Rules:          rules,
		},
		HistoryLimit:   s.config.HistoryLimit,
		TimeoutMinutes: s.config.TimeoutMinutes,
	}
}

// SetConfig 设置完整配置
func (s *Store) SetConfig(cfg RuntimeConfig) {
	s.mu.Lock()
	s.config = cfg
	fn := s.onUpdate
	s.mu.Unlock()

	if fn != nil {
		fn(&cfg)
	}
}

// --- Adapter CRUD ---

// ListAdapters 列出所有适配器
func (s *Store) ListAdapters() []adapter.AdapterConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]adapter.AdapterConfig, len(s.config.Adapters))
	copy(result, s.config.Adapters)
	return result
}

// GetAdapter 获取指定适配器
func (s *Store) GetAdapter(name string) *adapter.AdapterConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.config.Adapters {
		if s.config.Adapters[i].Name == name {
			cfg := s.config.Adapters[i]
			return &cfg
		}
	}
	return nil
}

// AddAdapter 添加适配器
func (s *Store) AddAdapter(cfg adapter.AdapterConfig) {
	s.mu.Lock()
	s.config.Adapters = append(s.config.Adapters, cfg)
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if fn != nil {
		fn(&configCopy)
	}
}

// UpdateAdapter 更新适配器
func (s *Store) UpdateAdapter(name string, cfg adapter.AdapterConfig) bool {
	s.mu.Lock()
	found := false
	for i := range s.config.Adapters {
		if s.config.Adapters[i].Name == name {
			s.config.Adapters[i] = cfg
			found = true
			break
		}
	}
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if found && fn != nil {
		fn(&configCopy)
	}
	return found
}

// DeleteAdapter 删除适配器
func (s *Store) DeleteAdapter(name string) bool {
	s.mu.Lock()
	found := false
	for i := range s.config.Adapters {
		if s.config.Adapters[i].Name == name {
			s.config.Adapters = append(s.config.Adapters[:i], s.config.Adapters[i+1:]...)
			found = true
			break
		}
	}
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if found && fn != nil {
		fn(&configCopy)
	}
	return found
}

// --- Routing ---

// GetRouting 获取路由配置
func (s *Store) GetRouting() router.RouterConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := make([]router.RouteRule, len(s.config.Routing.Rules))
	copy(rules, s.config.Routing.Rules)
	return router.RouterConfig{
		DefaultAdapter: s.config.Routing.DefaultAdapter,
		Rules:          rules,
	}
}

// SetRouting 更新路由配置
func (s *Store) SetRouting(routing router.RouterConfig) {
	s.mu.Lock()
	s.config.Routing = routing
	fn := s.onUpdate
	configCopy := s.config
	s.mu.Unlock()

	if fn != nil {
		fn(&configCopy)
	}
}
