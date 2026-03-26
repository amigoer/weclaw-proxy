package adapter

import (
	"os"
	"runtime"
	"strings"
	"time"
)

// ExpandPromptVars 展开系统提示词中的模板变量
// 支持的变量：
//   - {cur_date}     当前日期，格式 2006-01-02
//   - {cur_time}     当前时间，格式 15:04:05
//   - {cur_datetime} 完整日期时间，格式 2006-01-02 15:04:05
//   - {model_id}     当前使用的模型 ID
//   - {model_name}   模型名称（同 model_id）
//   - {locale}       系统语言环境（如 zh-CN、en-US）
func ExpandPromptVars(prompt string, modelID string) string {
	// 快速路径：不含变量标记时直接返回
	if !strings.Contains(prompt, "{") {
		return prompt
	}

	now := time.Now()
	replacer := strings.NewReplacer(
		"{cur_date}", now.Format("2006-01-02"),
		"{cur_time}", now.Format("15:04:05"),
		"{cur_datetime}", now.Format("2006-01-02 15:04:05"),
		"{model_id}", modelID,
		"{model_name}", modelID,
		"{locale}", detectLocale(),
	)
	return replacer.Replace(prompt)
}

// detectLocale 检测系统语言环境
func detectLocale() string {
	// 优先读取 LANG / LC_ALL 环境变量
	for _, env := range []string{"LC_ALL", "LANG", "LANGUAGE"} {
		if val := os.Getenv(env); val != "" {
			// 提取语言标签部分，如 "zh_CN.UTF-8" → "zh-CN"
			locale := strings.Split(val, ".")[0]
			locale = strings.ReplaceAll(locale, "_", "-")
			return locale
		}
	}

	// 根据操作系统返回默认值
	switch runtime.GOOS {
	case "darwin":
		return "zh-CN" // macOS 中文环境常见
	default:
		return "en-US"
	}
}
