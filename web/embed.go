// Package webui 通过 go:embed 将 React 面板构建产物打进 Hub 二进制，
// 使最终交付物为单文件（方案 3.2：部署无需额外前端资产目录）。
package webui

import "embed"

// dist 由 web/ 下 `npm run build` 生成；Dockerfile 与 Makefile 均先构建前端再编译 Go。
//
//go:embed all:dist
var DistFS embed.FS
