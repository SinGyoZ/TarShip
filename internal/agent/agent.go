// Package agent 为 TarShip 代理端：主动连出 Hub 长连接，接收指令并回报心跳。
// 服务器零入站端口（方案 3.1），断线按指数退避自动重连。
package agent

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/SinGyoZ/TarShip/internal/version"
)

const (
	defaultHeartbeat   = 30 * time.Second
	readTimeout        = 90 * time.Second // 与 Hub 一致：超时无消息即认为链路已死
	minBackoff         = time.Second
	maxBackoff         = time.Minute
	closeWriteDeadline = 3 * time.Second
)

// Run 解析 agent 子命令参数并进入连接循环（M0 验收项：子命令参数齐全）。
func Run(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	server := fs.String("server", "", "Hub 地址，如 wss://hub.example.com:8080（也接受 http(s):// 写法）")
	token := fs.String("token", "", "面板注册服务器时生成的 Agent token")
	hostname := fs.String("hostname", "", "上报的主机名（默认取操作系统主机名）")
	heartbeat := fs.Duration("heartbeat", defaultHeartbeat, "心跳间隔")
	_ = fs.Parse(args)

	if *server == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "必须提供 --server 与 --token 参数，示例：")
		fmt.Fprintln(os.Stderr, "  tarship agent --server wss://hub.example.com:8080 --token tsk_xxx")
		os.Exit(2)
	}
	host := strings.TrimSpace(*hostname)
	if host == "" {
		if h, err := os.Hostname(); err == nil {
			host = h
		} else {
			host = "unknown"
		}
	}
	if *heartbeat < 5*time.Second {
		log.Fatalf("[agent] 心跳间隔过短（最小 5s）: %v", *heartbeat)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := &client{server: *server, token: *token, hostname: host, heartbeat: *heartbeat}
	log.Printf("[agent] TarShip Agent v%s 启动，目标 %s", version.Version, *server)
	c.connectLoop(ctx)
	log.Println("[agent] 已退出")
}

type client struct {
	server    string
	token     string
	hostname  string
	heartbeat time.Duration
}

// connectLoop 持续维持与 Hub 的连接；每次会话失败后按指数退避重试。
func (c *client) connectLoop(ctx context.Context) {
	backoff := minBackoff
	for {
		err := c.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("[agent] 连接中断: %v；%s 后重连", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// session 建立一次连接直到断开；返回错误交由外层退避重连。
func (c *client) session(ctx context.Context) error {
	wsURL, err := agentWSURL(c.server)
	if err != nil {
		return err
	}
	header := http.Header{}
	// token 走请求头而非 URL，避免落在反代访问日志里
	header.Set("Authorization", "Bearer "+c.token)

	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(dialCtx, wsURL, header)
	if err != nil {
		return fmt.Errorf("拨号失败: %w", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type":          "hello",
		"agent_version": version.Version,
		"hostname":      c.hostname,
	}); err != nil {
		return fmt.Errorf("发送 hello: %w", err)
	}
	log.Printf("[agent] 已连接 %s（%s）", wsURL, c.hostname)

	send := make(chan any, 16)
	errCh := make(chan error, 2)
	go c.readPump(conn, send, errCh)
	go c.writePump(conn, send, errCh)

	select {
	case <-ctx.Done():
		// 收到退出信号：发 close 帧后静默退出，不触发重连
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "agent shutdown"),
			time.Now().Add(closeWriteDeadline))
		return nil
	case err := <-errCh:
		return err
	}
}

// readPump 只读；Hub 应用层 ping 在此应答 pong，保证双向链路活性检测。
func (c *client) readPump(conn *websocket.Conn, send chan<- any, errCh chan<- error) {
	var msg struct {
		Type string `json:"type"`
		TS   int64  `json:"ts"`
	}
	for {
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		if err := conn.ReadJSON(&msg); err != nil {
			errCh <- err
			return
		}
		if msg.Type == "ping" {
			select {
			case send <- map[string]any{"type": "pong", "ts": time.Now().Unix()}:
			default: // 发送队列满时丢弃本轮 pong，心跳仍会维持 Hub 侧活性
			}
		}
	}
}

// writePump 唯一写者：心跳定时与 pong 应答统一经此串行发送，规避并发写。
func (c *client) writePump(conn *websocket.Conn, send <-chan any, errCh chan<- error) {
	ticker := time.NewTicker(c.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case msg := <-send:
			if err := writeJSON(conn, msg); err != nil {
				errCh <- err
				return
			}
		case <-ticker.C:
			if err := writeJSON(conn, map[string]any{"type": "heartbeat", "ts": time.Now().Unix()}); err != nil {
				errCh <- err
				return
			}
		}
	}
}

func writeJSON(conn *websocket.Conn, v any) error {
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(v)
}

// agentWSURL 把 http(s) 归一化为 ws(s) 并拼上 Agent 长连接端点。
func agentWSURL(server string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(server), "/")
	switch {
	case strings.HasPrefix(s, "wss://"), strings.HasPrefix(s, "ws://"):
	case strings.HasPrefix(s, "https://"):
		s = "wss://" + strings.TrimPrefix(s, "https://")
	case strings.HasPrefix(s, "http://"):
		s = "ws://" + strings.TrimPrefix(s, "http://")
	default:
		return "", fmt.Errorf("--server 需以 ws:// wss:// http:// https:// 开头: %q", server)
	}
	return s + "/ws/agent", nil
}
