// tarship 是 TarShip 的单一可执行文件：hub 与 agent 两种角色由子命令区分。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/SinGyoZ/TarShip/internal/agent"
	"github.com/SinGyoZ/TarShip/internal/config"
	"github.com/SinGyoZ/TarShip/internal/hub"
	"github.com/SinGyoZ/TarShip/internal/version"
)

const usage = `TarShip - 目录备份系统

用法:
  tarship hub          启动 Hub（控制面 + 面板）
  tarship agent        启动 Agent（部署在被备份的服务器上）
  tarship healthcheck  检查本机 Hub 是否健康（供 Docker HEALTHCHECK 使用）
  tarship reindex      从存储器重建索引（M4 实现）
  tarship version      显示版本

各子命令加 -h 查看参数。`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	args := os.Args[2:]

	var err error
	switch os.Args[1] {
	case "hub":
		err = runHub(ctx, args, log)
	case "agent":
		err = runAgent(ctx, args, log)
	case "healthcheck":
		err = runHealthcheck(args)
	case "reindex":
		err = errors.New("reindex 将在 M4 实现")
	case "version", "-v", "--version":
		fmt.Printf("tarship %s (%s %s/%s)\n", version.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	case "help", "-h", "--help":
		fmt.Println(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n\n%s\n", os.Args[1], usage)
		os.Exit(2)
	}

	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "tarship:", err)
		os.Exit(1)
	}
}

func runHub(ctx context.Context, args []string, log *slog.Logger) error {
	cfg, err := config.ParseHub(args, os.Stderr, os.Getenv)
	if err != nil {
		return err
	}
	return hub.Run(ctx, hub.Config{Addr: cfg.Addr, DataDir: cfg.DataDir, TrustProxy: cfg.TrustProxy}, log)
}

func runAgent(ctx context.Context, args []string, log *slog.Logger) error {
	cfg, err := config.ParseAgent(args, os.Stderr, os.Getenv)
	if err != nil {
		return err
	}
	if cfg.TokenFromFlag {
		log.Warn("token 通过命令行参数传入，会出现在进程列表里；建议改用环境变量 TARSHIP_TOKEN")
	}
	a, err := agent.New(agent.Config{Server: cfg.Server, Token: cfg.Token, AllowInsecureWS: cfg.AllowInsecureWS}, log)
	if err != nil {
		return err
	}
	log.Info("Agent 启动", "version", version.Version)
	return a.Run(ctx)
}

func runHealthcheck(args []string) error {
	fs := flag.NewFlagSet("tarship healthcheck", flag.ContinueOnError)
	url := fs.String("url", config.HealthURL(envOr("TARSHIP_ADDR", ":8080")), "健康检查地址")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(*url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("健康检查失败: HTTP %d", resp.StatusCode)
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
