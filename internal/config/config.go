// Package config 解析 hub / agent 子命令的命令行参数与环境变量。
// 优先级：命令行参数 > 环境变量 > 内置默认值。
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

// Getenv 抽象环境变量读取，便于测试。
type Getenv func(string) string

// Hub 是 hub 子命令的配置。
type Hub struct {
	Addr       string
	DataDir    string
	TrustProxy bool
}

// Agent 是 agent 子命令的配置。
type Agent struct {
	Server          string
	Token           string
	AllowInsecureWS bool
	// TokenFromFlag 为 true 时，token 来自命令行参数，会出现在进程列表里。
	TokenFromFlag bool
}

func envOr(getenv Getenv, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(getenv Getenv, key string) (bool, error) {
	v := getenv(key)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("环境变量 %s 不是合法的布尔值: %q", key, v)
	}
	return b, nil
}

// ParseHub 解析 `tarship hub` 的参数。
func ParseHub(args []string, stderr io.Writer, getenv Getenv) (Hub, error) {
	trust, err := envBool(getenv, "TARSHIP_TRUST_PROXY")
	if err != nil {
		return Hub{}, err
	}
	cfg := Hub{
		Addr:       envOr(getenv, "TARSHIP_ADDR", ":8080"),
		DataDir:    envOr(getenv, "TARSHIP_DATA", "/data"),
		TrustProxy: trust,
	}
	fs := flag.NewFlagSet("tarship hub", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "监听地址 [TARSHIP_ADDR]")
	fs.StringVar(&cfg.DataDir, "data", cfg.DataDir, "数据目录，存放 SQLite 与配置 [TARSHIP_DATA]")
	fs.BoolVar(&cfg.TrustProxy, "trust-proxy", cfg.TrustProxy, "信任反向代理的 X-Forwarded-For 来获取客户端 IP，仅在 Hub 前面确有可信反代时开启 [TARSHIP_TRUST_PROXY]")
	if err := fs.Parse(args); err != nil {
		return Hub{}, err
	}
	if fs.NArg() > 0 {
		return Hub{}, fmt.Errorf("多余的参数: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

// ParseAgent 解析 `tarship agent` 的参数。
func ParseAgent(args []string, stderr io.Writer, getenv Getenv) (Agent, error) {
	insecure, err := envBool(getenv, "TARSHIP_ALLOW_INSECURE_WS")
	if err != nil {
		return Agent{}, err
	}
	cfg := Agent{
		Server:          getenv("TARSHIP_SERVER"),
		Token:           getenv("TARSHIP_TOKEN"),
		AllowInsecureWS: insecure,
	}
	fs := flag.NewFlagSet("tarship agent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.Server, "server", cfg.Server, "Hub 地址，例如 wss://hub.example.com [TARSHIP_SERVER]")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "注册 token；更推荐用环境变量 TARSHIP_TOKEN，避免出现在进程列表里")
	fs.BoolVar(&cfg.AllowInsecureWS, "allow-insecure-ws", cfg.AllowInsecureWS, "允许明文 ws:// 连接，仅用于本地调试 [TARSHIP_ALLOW_INSECURE_WS]")
	if err := fs.Parse(args); err != nil {
		return Agent{}, err
	}
	if fs.NArg() > 0 {
		return Agent{}, fmt.Errorf("多余的参数: %s", strings.Join(fs.Args(), " "))
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "token" {
			cfg.TokenFromFlag = true
		}
	})
	if strings.TrimSpace(cfg.Server) == "" {
		return Agent{}, errors.New("缺少 Hub 地址：请使用 --server 或环境变量 TARSHIP_SERVER")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return Agent{}, errors.New("缺少 token：请使用环境变量 TARSHIP_TOKEN（或 --token）")
	}
	return cfg, nil
}

// HealthURL 根据 Hub 监听地址推导本机健康检查地址。
func HealthURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://127.0.0.1:8080/healthz"
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz"
}
