package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/amigoer/weclaw-proxy/internal/adapter"
	"github.com/amigoer/weclaw-proxy/internal/router"
	"github.com/amigoer/weclaw-proxy/internal/session"
	"gopkg.in/yaml.v3"
)

// Config 应用全局配置
type Config struct {
	Server   ServerConfig           `yaml:"server"`
	Weixin   WeixinConfig           `yaml:"weixin"`
	Adapters []adapter.AdapterConfig `yaml:"adapters"`
	Routing  router.RouterConfig    `yaml:"routing"`
	Session  session.ManagerConfig  `yaml:"session"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host,omitempty"`
}

// WeixinConfig 微信连接配置
type WeixinConfig struct {
	BaseURL           string `yaml:"base_url"`
	CDNBaseURL        string `yaml:"cdn_base_url"`
	LongPollTimeoutMs int    `yaml:"long_poll_timeout_ms"`
	DataDir           string `yaml:"data_dir"` // 数据持久化目录
}

// Load 从 YAML 文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 展开环境变量
	content := os.ExpandEnv(string(data))

	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(content), cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	cfg.applyDefaults()

	return cfg, nil
}

// applyDefaults 填充默认值
func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Weixin.BaseURL == "" {
		c.Weixin.BaseURL = "https://ilinkai.weixin.qq.com"
	}
	if c.Weixin.CDNBaseURL == "" {
		c.Weixin.CDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	}
	if c.Weixin.LongPollTimeoutMs == 0 {
		c.Weixin.LongPollTimeoutMs = 35000
	}
	if c.Weixin.DataDir == "" {
		c.Weixin.DataDir = ".weclaw-data"
	}
	if c.Session.HistoryLimit == 0 {
		c.Session.HistoryLimit = 20
	}
	if c.Session.TimeoutMinutes == 0 {
		c.Session.TimeoutMinutes = 30
	}
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	if len(c.Adapters) == 0 {
		return fmt.Errorf("至少需要配置一个适配器")
	}

	adapterNames := make(map[string]bool)
	for _, a := range c.Adapters {
		if a.Name == "" {
			return fmt.Errorf("适配器名称不能为空")
		}
		if adapterNames[a.Name] {
			return fmt.Errorf("适配器名称重复: %s", a.Name)
		}
		adapterNames[a.Name] = true

		validTypes := map[string]bool{
			"openai": true, "anthropic": true, "dify": true,
			"coze": true, "webhook": true,
		}
		if !validTypes[a.AdapterType] {
			return fmt.Errorf("不支持的适配器类型: %s（支持: %s）",
				a.AdapterType,
				strings.Join([]string{"openai", "anthropic", "dify", "coze", "webhook"}, ", "),
			)
		}
	}

	// 验证默认适配器存在
	if c.Routing.DefaultAdapter != "" {
		if !adapterNames[c.Routing.DefaultAdapter] {
			return fmt.Errorf("默认适配器 '%s' 不在已配置的适配器列表中", c.Routing.DefaultAdapter)
		}
	}

	// 验证路由规则引用的适配器存在
	for i, rule := range c.Routing.Rules {
		if !adapterNames[rule.AdapterName] {
			return fmt.Errorf("路由规则 #%d 引用的适配器 '%s' 不存在", i+1, rule.AdapterName)
		}
	}

	return nil
}
