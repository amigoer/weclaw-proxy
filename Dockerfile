# 阶段1：构建前端
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 阶段2：构建 Go 二进制
FROM golang:1.25-alpine AS backend
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/internal/server/dist/ ./internal/server/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o weclaw-proxy ./cmd/weclaw-proxy/

# 阶段3：最终镜像
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend /app/weclaw-proxy /usr/local/bin/weclaw-proxy

# 创建数据目录
RUN mkdir -p /data

EXPOSE 8080

ENTRYPOINT ["weclaw-proxy"]
CMD ["--config", "/data/config.yaml"]
