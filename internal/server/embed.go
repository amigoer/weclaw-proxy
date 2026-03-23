package server

import "embed"

// FrontendDist 嵌入前端构建产物
//
//go:embed all:dist
var FrontendDist embed.FS
