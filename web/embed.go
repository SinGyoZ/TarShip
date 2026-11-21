// Package web 内嵌前端构建产物（web/dist），让 Hub 成为单个二进制。
package web

import "embed"

// all: 前缀让 dist 里只有 .gitkeep 时（尚未构建前端）也能通过编译。
//
//go:embed all:dist
var Dist embed.FS
