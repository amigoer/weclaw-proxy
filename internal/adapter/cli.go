package adapter

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CLI 适配器
// 通过调用本地 CLI 工具（如 codex exec / claude -p / gemini -p）对接 AI Agent

// CLIAdapter CLI 命令行适配器
type CLIAdapter struct {
	name    string
	command string   // CLI 命令路径（如 codex, claude, gemini）
	args    []string // 子命令参数（如 exec, -p）
	workDir string   // 工作目录（可选）
	timeout time.Duration
	logger  *slog.Logger
}

// NewCLIAdapter 创建 CLI 适配器
func NewCLIAdapter(cfg *AdapterConfig, logger *slog.Logger) *CLIAdapter {
	if logger == nil {
		logger = slog.Default()
	}

	command := cfg.BaseURL
	if command == "" {
		command = "echo" // 默认回显（用于测试）
	}

	// 解析子命令参数
	var args []string
	if rawArgs := cfg.Extra["args"]; rawArgs != "" {
		args = strings.Fields(rawArgs)
	} else {
		// 根据命令名自动推断默认参数
		base := strings.TrimSuffix(command, ".exe")
		switch {
		case strings.HasSuffix(base, "codex"):
			args = []string{"exec"}
		case strings.HasSuffix(base, "claude"):
			args = []string{"-p"}
		case strings.HasSuffix(base, "gemini"):
			args = []string{"-p"}
		}
	}

	// 超时时间，默认 120 秒
	timeout := 120 * time.Second
	if t := cfg.Extra["timeout"]; t != "" {
		if seconds, err := strconv.Atoi(t); err == nil && seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	workDir := cfg.Extra["work_dir"]

	return &CLIAdapter{
		name:    cfg.Name,
		command: command,
		args:    args,
		workDir: workDir,
		timeout: timeout,
		logger:  logger,
	}
}

func (a *CLIAdapter) Name() string { return a.name }
func (a *CLIAdapter) Type() string { return "cli" }

// Chat 同步对话 — 执行 CLI 命令并返回 stdout
func (a *CLIAdapter) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 构建命令参数：子命令 + 用户消息
	cmdArgs := make([]string, 0, len(a.args)+1)
	cmdArgs = append(cmdArgs, a.args...)
	cmdArgs = append(cmdArgs, req.Message)

	// 设置超时
	execCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, a.command, cmdArgs...)

	if a.workDir != "" {
		cmd.Dir = a.workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	a.logger.Info("执行 CLI 命令",
		"command", a.command,
		"args", cmdArgs,
		"timeout", a.timeout.String(),
	)

	startTime := time.Now()
	err := cmd.Run()
	elapsed := time.Since(startTime)

	a.logger.Info("CLI 命令执行完成",
		"elapsed", elapsed.String(),
		"exitCode", cmd.ProcessState.ExitCode(),
		"stdoutLen", stdout.Len(),
		"stderrLen", stderr.Len(),
	)

	if err != nil {
		// 超时处理
		if execCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("cli: 命令执行超时 (%s)", a.timeout.String())
		}
		// 非零退出码但有 stdout 输出时，仍返回结果（部分 CLI 工具用非零退出码表示警告）
		if stdout.Len() > 0 {
			a.logger.Warn("CLI 命令非零退出但有输出，返回 stdout",
				"error", err,
				"stderr", stderr.String(),
			)
		} else {
			errMsg := stderr.String()
			if errMsg == "" {
				errMsg = err.Error()
			}
			return nil, fmt.Errorf("cli: 命令执行失败: %s", errMsg)
		}
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = "(CLI 无输出)"
	}

	return &ChatResponse{
		Text: output,
	}, nil
}

// ChatStream CLI 不支持流式，回退到同步模式
func (a *CLIAdapter) ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error) {
	resp, err := a.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *ChatChunk, 1)
	go func() {
		defer close(ch)
		ch <- &ChatChunk{
			Text: resp.Text,
			Done: true,
		}
	}()

	return ch, nil
}
