package hub

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// PanelHub 向所有已认证的面板连接广播服务器状态事件；
// 面板收到事件后自行失效查询缓存，实现"实时在线状态"而无需轮询。
type PanelHub struct {
	auth *authService

	// snapshot 返回当前在线 Agent 快照；绑定发生在 Hub 装配阶段、首个连接之前
	snapshot func() map[int64]onlineInfo

	mu    sync.Mutex
	conns map[*websocket.Conn]struct{}
}

func newPanelHub(auth *authService) *PanelHub {
	return &PanelHub{auth: auth, conns: make(map[*websocket.Conn]struct{})}
}

// bindSnapshot 注入在线状态提供方（避免与 AgentHub 循环依赖）。
func (p *PanelHub) bindSnapshot(fn func() map[int64]onlineInfo) {
	p.snapshot = fn
}

// panelEvent 面板侧事件载荷。M0 事件：server_online / server_offline / server_removed。
type panelEvent struct {
	Type string     `json:"type"`
	Data onlineInfo `json:"data,omitempty"`
}

func (p *PanelHub) HandlePanelWS(w http.ResponseWriter, r *http.Request) {
	// 浏览器 WebSocket 无法携带自定义头，但会带上会话 Cookie，据此鉴权
	if _, err := p.auth.sessionUser(r); err != nil {
		httpError(w, http.StatusUnauthorized, "未登录或会话已过期")
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	p.mu.Lock()
	p.conns[conn] = struct{}{}
	p.mu.Unlock()
	// 注册回执：新开面板页立刻获得当前在线快照，无需等待下一个事件
	online := make([]onlineInfo, 0)
	if p.snapshot != nil {
		for _, info := range p.snapshot() {
			online = append(online, info)
		}
	}
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err := conn.WriteJSON(map[string]any{"type": "panel_hello", "online": online}); err != nil {
		p.remove(conn)
		return
	}
	go p.readPump(conn)
}

// readPump 只负责感知断开：面板不向 Hub 发消息，读到任何内容或错误即清理。
func (p *PanelHub) readPump(conn *websocket.Conn) {
	defer p.remove(conn)
	for {
		// 读超时设长一些；面板刷新页面即可重连，这里只兜底清理死连接
		_ = conn.SetReadDeadline(time.Now().Add(24 * time.Hour))
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (p *PanelHub) remove(conn *websocket.Conn) {
	p.mu.Lock()
	delete(p.conns, conn)
	p.mu.Unlock()
	conn.Close()
}

// broadcast 向全部面板连接推送事件；单连接写失败只影响该连接本身。
func (p *PanelHub) broadcast(ev panelEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("[panel] 序列化事件失败: %v", err)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for conn := range p.conns {
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			// 写失败即视为死连接，删除动作 defer 由锁内直接执行
			delete(p.conns, conn)
			conn.Close()
		}
	}
}

func (p *PanelHub) broadcastOnline(info onlineInfo) {
	p.broadcast(panelEvent{Type: "server_online", Data: info})
}

func (p *PanelHub) broadcastOffline(serverID int64) {
	p.broadcast(panelEvent{Type: "server_offline", Data: onlineInfo{ServerID: serverID}})
}

func (p *PanelHub) broadcastRemoved(serverID int64) {
	p.broadcast(panelEvent{Type: "server_removed", Data: onlineInfo{ServerID: serverID}})
}
