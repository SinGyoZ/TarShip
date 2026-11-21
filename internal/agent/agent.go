// Package agent 实现部署在被备份服务器上的 Agent：出站连接 Hub、心跳、断线重连。
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/SinGyoZ/TarShip/internal/proto"
	"github.com/SinGyoZ/TarShip/internal/version"
)

// Config 是 Agent 的运行配置。
type Config struct {
	Server          string
	Token           string
	AllowInsecureWS bool
}

// Agent 维持与 Hub 的长连接。
type Agent struct {
	cfg      Config
	url      string
	hostname string
	log      *slog.Logger
}

// fatalError 表示重试也没有意义的错误（例如被同 token 的另一个 Agent 顶替）。
type fatalError struct{ msg string }

func (e *fatalError) Error() string { return e.msg }

// New 校验配置并创建 Agent。
func New(cfg Config, log *slog.Logger) (*Agent, error) {
	u, err := WSURL(cfg.Server, cfg.AllowInsecureWS)
	if err != nil {
		return nil, err
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return &Agent{cfg: cfg, url: u, hostname: host, log: log}, nil
}

// WSURL 把 --server 参数规范化为 Agent 连接地址。默认只接受 wss://，
// 因为 token 会在握手头里传输，明文连接等于把它交给链路上的任何人。
func WSURL(server string, allowInsecure bool) (string, error) {
	u, err := url.Parse(server)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("Hub 地址不合法: %q，示例: wss://hub.example.com", server)
	}
	switch u.Scheme {
	case "wss":
	case "ws":
		if !allowInsecure {
			return "", errors.New("拒绝使用明文 ws:// 连接（token 会被明文传输）；仅本地调试可加 --allow-insecure-ws")
		}
	default:
		return "", fmt.Errorf("Hub 地址必须以 wss:// 开头，当前是 %q", server)
	}
	return u.JoinPath(proto.AgentWSPath).String(), nil
}

// Run 保持连接直到 ctx 取消；遇到致命错误时返回它。
func (a *Agent) Run(ctx context.Context) error {
	bo := newBackoff(time.Second, time.Minute)
	for {
		started := time.Now()
		err := a.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		var fe *fatalError
		if errors.As(err, &fe) {
			return fe
		}
		if time.Since(started) >= time.Minute {
			bo.reset() // 稳定连接过一段时间，说明之前的故障已恢复，退避从头开始
		}
		d := bo.next()
		a.log.Warn("与 Hub 的连接已断开，稍后重试", "error", err, "retry_in", d.Round(time.Millisecond))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(d):
		}
	}
}

func (a *Agent) session(ctx context.Context) error {
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	conn, resp, err := websocket.Dial(dctx, a.url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + a.cfg.Token}},
	})
	cancel()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return errors.New("Hub 拒绝了该 token（401）：请确认服务器未被删除、token 未被重置")
		}
		return fmt.Errorf("连接 Hub 失败: %w", err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(proto.MaxMessageBytes)

	sctx, scancel := context.WithCancel(ctx)
	defer scancel()

	hello := proto.Hello{
		Type: proto.TypeHello, AgentVersion: version.Version,
		Hostname: a.hostname, ProtocolVersion: proto.Version,
	}
	if err := writeMsg(sctx, conn, hello); err != nil {
		return fmt.Errorf("发送 hello 失败: %w", err)
	}

	msgs := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		for {
			rctx, rcancel := context.WithTimeout(sctx, proto.OfflineTimeout)
			_, data, err := conn.Read(rctx)
			rcancel()
			if err != nil {
				readErr <- err
				return
			}
			select {
			case msgs <- data:
			case <-sctx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(proto.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "agent exiting")
			return ctx.Err()
		case err := <-readErr:
			return classifyReadError(err)
		case data := <-msgs:
			a.handle(data)
		case <-ticker.C:
			hb := proto.Heartbeat{Type: proto.TypeHeartbeat, TS: time.Now().Unix()}
			if err := writeMsg(sctx, conn, hb); err != nil {
				return fmt.Errorf("发送心跳失败: %w", err)
			}
		}
	}
}

func (a *Agent) handle(data []byte) {
	typ, err := proto.PeekType(data)
	if err != nil {
		return
	}
	switch typ {
	case proto.TypeWelcome:
		a.log.Info("已连接到 Hub", "hostname", a.hostname)
	case proto.TypeHeartbeatAck:
	default:
		a.log.Debug("忽略未知消息类型", "type", typ)
	}
}

func classifyReadError(err error) error {
	switch websocket.CloseStatus(err) {
	case proto.CloseReplaced:
		return &fatalError{"同一 token 已被另一个 Agent 使用并顶替了本连接：请确认没有重复部署"}
	case proto.CloseProtocolMismatch:
		return &fatalError{"Agent 与 Hub 的协议版本不兼容：请把两端升级到同一版本"}
	}
	return fmt.Errorf("连接中断: %w", err)
}

func writeMsg(ctx context.Context, conn *websocket.Conn, v any) error {
	wctx, cancel := context.WithTimeout(ctx, proto.WriteTimeout)
	defer cancel()
	return wsjson.Write(wctx, conn, v)
}

// backoff 实现带抖动的指数退避。
type backoff struct {
	base, max, cur time.Duration
}

func newBackoff(base, max time.Duration) *backoff { return &backoff{base: base, max: max} }

func (b *backoff) next() time.Duration {
	d := b.cur
	if d == 0 {
		d = b.base
	}
	b.cur = min(d*2, b.max)
	// ±20% 抖动，避免 Hub 重启后所有 Agent 同一时刻一起重连。
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
}

func (b *backoff) reset() { b.cur = 0 }
