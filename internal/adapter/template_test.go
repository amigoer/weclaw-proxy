package adapter

import (
	"strings"
	"testing"
	"time"
)

func TestExpandPromptVars_NoVars(t *testing.T) {
	// 不含变量的提示词应原样返回
	input := "你是一个友好的助手"
	result := ExpandPromptVars(input, "gpt-4o")
	if result != input {
		t.Errorf("期望原样返回 %q，实际得到 %q", input, result)
	}
}

func TestExpandPromptVars_DateVars(t *testing.T) {
	input := "今天是 {cur_date}，时间 {cur_time}，完整 {cur_datetime}"
	result := ExpandPromptVars(input, "gpt-4o")

	today := time.Now().Format("2006-01-02")
	if !strings.Contains(result, today) {
		t.Errorf("期望包含日期 %q，实际: %q", today, result)
	}

	// 不应包含原始变量标记
	for _, v := range []string{"{cur_date}", "{cur_time}", "{cur_datetime}"} {
		if strings.Contains(result, v) {
			t.Errorf("未替换变量 %q，结果: %q", v, result)
		}
	}
}

func TestExpandPromptVars_ModelVars(t *testing.T) {
	input := "你正在使用 {model_id} 模型，名称为 {model_name}"
	result := ExpandPromptVars(input, "deepseek-v3")

	expected := "你正在使用 deepseek-v3 模型，名称为 deepseek-v3"
	if result != expected {
		t.Errorf("期望 %q，实际 %q", expected, result)
	}
}

func TestExpandPromptVars_LocaleVar(t *testing.T) {
	input := "请用 {locale} 回答"
	result := ExpandPromptVars(input, "gpt-4o")

	if strings.Contains(result, "{locale}") {
		t.Errorf("未替换 {locale}，结果: %q", result)
	}
	// locale 应不为空
	if result == "请用  回答" {
		t.Errorf("locale 替换为空值")
	}
}

func TestExpandPromptVars_MixedContent(t *testing.T) {
	input := "日期 {cur_date}，模型 {model_id}，普通文本不变 {unknown_var}"
	result := ExpandPromptVars(input, "gpt-4o")

	// 已知变量应被替换
	if strings.Contains(result, "{cur_date}") {
		t.Errorf("未替换 {cur_date}")
	}
	if strings.Contains(result, "{model_id}") {
		t.Errorf("未替换 {model_id}")
	}
	// 未知变量应保留原样
	if !strings.Contains(result, "{unknown_var}") {
		t.Errorf("未知变量不应被替换: %q", result)
	}
}

func TestExpandPromptVars_EmptyPrompt(t *testing.T) {
	result := ExpandPromptVars("", "gpt-4o")
	if result != "" {
		t.Errorf("空字符串应返回空，实际: %q", result)
	}
}
