.PHONY: all build web clean dev dev-web login help

# 默认目标：构建前端 + Go 二进制
all: build

# 完整构建
build: web
	go build -o weclaw-proxy ./cmd/weclaw-proxy/
	@echo "✅ 构建完成: ./weclaw-proxy"

# 仅构建前端
web:
	cd web && npm install --silent && npm run build

# 仅构建 Go（跳过前端，适用于后端改动）
go:
	go build -o weclaw-proxy ./cmd/weclaw-proxy/

# 启动开发模式（Go 后端）
dev:
	go run ./cmd/weclaw-proxy/ --config configs/config.yaml --verbose

# 启动前端开发服务器（热更新）
dev-web:
	cd web && npm run dev

# 微信登录
login:
	go run ./cmd/weclaw-proxy/ --login --config configs/config.yaml

# 代码检查
lint:
	go vet ./...
	cd web && npx tsc --noEmit

# 清理构建产物
clean:
	rm -f weclaw-proxy
	rm -rf internal/server/dist

# 帮助
help:
	@echo "WeClaw-Proxy Makefile"
	@echo ""
	@echo "  make          - 构建前端 + Go 二进制"
	@echo "  make web      - 仅构建前端"
	@echo "  make go       - 仅构建 Go 后端"
	@echo "  make dev      - 启动 Go 开发模式"
	@echo "  make dev-web  - 启动前端热更新服务器"
	@echo "  make login    - 微信扫码登录"
	@echo "  make lint     - 代码检查"
	@echo "  make clean    - 清理构建产物"
