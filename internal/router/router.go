package router

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/amigoer/weclaw-proxy/internal/adapter"
)

// MatchRule 路由匹配规则
type MatchRule struct {
	Prefix  string   `yaml:"prefix,omitempty"`   // 消息前缀匹配（如 "/gpt"）
	UserIDs []string `yaml:"user_ids,omitempty"` // 用户 ID 匹配列表
}

// RouteRule 路由规则
type RouteRule struct {
	Match       MatchRule `yaml:"match"`
	AdapterName string    `yaml:"adapter"` // 目标适配器名称
}

// RouterConfig 路由配置
type RouterConfig struct {
	DefaultAdapter string      `yaml:"default_adapter"` // 默认适配器名称
	Rules          []RouteRule `yaml:"rules"`           // 路由规则列表
}

// Router 消息路由器
type Router struct {
	adapters       map[string]adapter.Adapter // 适配器注册表
	defaultAdapter string                     // 默认适配器名称
	rules          []RouteRule                // 路由规则
	mu             sync.RWMutex
	logger         *slog.Logger
}

// NewRouter 创建新的消息路由器
func NewRouter(cfg *RouterConfig, logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Router{
		adapters: make(map[string]adapter.Adapter),
		logger:   logger,
	}
	if cfg != nil {
		r.defaultAdapter = cfg.DefaultAdapter
		r.rules = cfg.Rules
	}
	return r
}

// RegisterAdapter 注册适配器
func (r *Router) RegisterAdapter(a adapter.Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Name()] = a
	r.logger.Info("适配器已注册", "name", a.Name(), "type", a.Type())
}

// GetAdapter 获取指定名称的适配器
func (r *Router) GetAdapter(name string) (adapter.Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	return a, ok
}

// Route 根据消息内容和用户 ID 路由到合适的适配器
// 返回：(适配器, 去掉前缀后的消息文本, 错误)
func (r *Router) Route(userID string, message string) (adapter.Adapter, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 按规则顺序匹配
	for _, rule := range r.rules {
		// 检查前缀匹配
		if rule.Match.Prefix != "" {
			if strings.HasPrefix(message, rule.Match.Prefix) {
				a, ok := r.adapters[rule.AdapterName]
				if !ok {
					r.logger.Warn("路由规则匹配但适配器不存在",
						"prefix", rule.Match.Prefix,
						"adapter", rule.AdapterName,
					)
					continue
				}
				// 去掉前缀，并去除前导空格
				cleanMsg := strings.TrimSpace(strings.TrimPrefix(message, rule.Match.Prefix))
				r.logger.Debug("路由匹配: 前缀",
					"prefix", rule.Match.Prefix,
					"adapter", rule.AdapterName,
				)
				return a, cleanMsg, nil
			}
		}

		// 检查用户 ID 匹配
		if len(rule.Match.UserIDs) > 0 {
			for _, uid := range rule.Match.UserIDs {
				if uid == userID {
					a, ok := r.adapters[rule.AdapterName]
					if !ok {
						r.logger.Warn("路由规则匹配但适配器不存在",
							"userID", userID,
							"adapter", rule.AdapterName,
						)
						continue
					}
					r.logger.Debug("路由匹配: 用户ID",
						"userID", userID,
						"adapter", rule.AdapterName,
					)
					return a, message, nil
				}
			}
		}
	}

	// 使用默认适配器
	if r.defaultAdapter != "" {
		a, ok := r.adapters[r.defaultAdapter]
		if ok {
			return a, message, nil
		}
	}

	return nil, message, fmt.Errorf("没有匹配的适配器: userID=%s", userID)
}

// UpdateRules 动态更新路由规则
func (r *Router) UpdateRules(rules []RouteRule, defaultAdapter string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules = rules
	if defaultAdapter != "" {
		r.defaultAdapter = defaultAdapter
	}
	r.logger.Info("路由规则已更新", "ruleCount", len(rules), "default", r.defaultAdapter)
}

// ListAdapters 列出所有已注册的适配器
func (r *Router) ListAdapters() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}
