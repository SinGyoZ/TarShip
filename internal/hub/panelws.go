package hub

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"net/http"
)

const panelPingInterval = 25 * time.Second // 小于常见反向代理 60s 的空闲超时

// panelBroker 把事件广播给所有打开的面板页面。
type panelBroker struct {
	mu   sync.Mutex
	subs map[*panelSub]struct{}
}

type panelSub struct {
	ch       chan []byte
	overflow chan struct{}
}

func newPanelBroker() *panelBroker {
	return &panelBroker{subs: map[*panelSub]struct{}{}}
}

func (b *panelBroker) subscribe() *panelSub {
	s := &panelSub{ch: make(chan []byte, 64), overflow: make(chan struct{}, 1)}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

func (b *panelBroker) unsubscribe(s *panelSub) {
	b.mu.Lock()
	delete(b.subs, s)
	b.mu.Unlock()
}

// publish 非阻塞广播；某个页面消费太慢时通知它断开重连（重连后会收到完整快照）。
func (b *panelBroker) publish(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		select {
		case s.ch <- data:
		default:
			select {
			case s.overflow <- struct{}{}:
			default:
			}
		}
	}
}

type statusEvent struct {
	Type         string `json:"type"`
	ServerID     int64  `json:"server_id"`
	Online       bool   `json:"online"`
	Hostname     string `json:"hostname,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`
	LastSeenAt   string `json:"last_seen_at"`
}

func (h *Hub) publishStatus(id int64, online bool, hostname, version string, seen time.Time) {
	h.panel.publish(statusEvent{
		Type: "server_status", ServerID: id, Online: online,
		Hostname: hostname, AgentVersion: version, LastSeenAt: seen.UTC().Format(time.RFC3339Nano),
	})
}

func (h *Hub) publishServersChanged() {
	h.panel.publish(map[string]string{"type": "servers_changed"})
}

func (h *Hub) handlePanelWS(w http.ResponseWriter, r *http.Request) {
	// Accept 默认要求 Origin 与 Host 同源，浏览器跨站发起的 WebSocket 会被拒绝。
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	h.wg.Add(1)
	defer h.wg.Done()
	defer c.CloseNow()

	ctx := c.CloseRead(h.ctx)
	// 先订阅再取快照，避免两者之间的事件丢失。
	sub := h.panel.subscribe()
	defer h.panel.unsubscribe(sub)

	servers, err := h.listDTOs(ctx)
	if err != nil {
		h.log.Error("面板快照失败", "error", err)
		c.Close(websocket.StatusInternalError, "internal error")
		return
	}
	snapshot, _ := json.Marshal(map[string]any{"type": "snapshot", "servers": servers})
	if !writeText(ctx, c, snapshot) {
		return
	}

	ping := time.NewTicker(panelPingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			c.Close(websocket.StatusGoingAway, "")
			return
		case msg := <-sub.ch:
			if !writeText(ctx, c, msg) {
				return
			}
		case <-sub.overflow:
			c.Close(websocket.StatusTryAgainLater, "消息积压，请重连")
			return
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func writeText(ctx context.Context, c *websocket.Conn, data []byte) bool {
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.Write(wctx, websocket.MessageText, data) == nil
}
