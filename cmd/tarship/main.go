// TarShip 单二进制双角色入口：靠子命令区分 Hub（控制面）与 Agent（代理端）。
package main

import (
	"fmt"
	"os"

	"github.com/SinGyoZ/TarShip/internal/agent"
	"github.com/SinGyoZ/TarShip/internal/hub"
	"github.com/SinGyoZ/TarShip/internal/version"
)

const usageText = `tarship - 目录备份运输系统（单二进制，双角色）

用法:
  tarship hub    启动控制面（调度 + REST/WebSocket API + SQLite + 内嵌面板）
  tarship agent  启动代理端（主动连出 Hub，长连接接收指令）
  tarship version  打印版本号
  tarship help    显示本帮助

示例:
  tarship hub --addr :8080 --data /data
  tarship agent --server ws://hub.example.com:8080 --token <TOKEN>

完整文档: https://github.com/SinGyoZ/TarShip
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "hub":
		hub.Run(os.Args[2:])
	case "agent":
		agent.Run(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("tarship", version.Version)
	case "help", "-h", "--help":
		fmt.Print(usageText)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %q\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
}
