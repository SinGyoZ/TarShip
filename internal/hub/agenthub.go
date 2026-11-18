package hub

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 与方案 5.1 一致的心跳与离线判定参数。
const (
	heartbeatInterval = 30 * time.Second
	readTimeout       = 90 * time.Second // 超过该时长无任何消息即判定离线
	agentPingInterval = 45 * time.Second // Hub 侧主动 ping，供 Agent 检测半开连接
	writeTimeout      = 10 * time.Second
)

// agentMsg 为 M0 阶段 Agent → Hub 的消息集合（JSON over WebSocket）。
type agentMsg struct {
	Type         string `json:"type"`
	AgentVersion string `json:"agent_version,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	TS           int64  `json:"ts,omitempty"`
}

// agentConn 表示一台已上线 Agent 的连接。
type agentConn struct {
	serverID  int64
	conn      *websocket.Conn
	hostname  string
	version   string
	announced bool // 收到 hello 后才广播上线，避免空载荷事件
}

// AgentHub 管理 Agent 长连接：注册认证、在线表、离线判定与面板广播。
type AgentHub struct {
	store *store
	panel *PanelHub

	mu    sync.Mutex
	conns map[int64]*agentConn // server_id -> 当前连接；同 token 重连时踢旧留新
}

func newAgentHub(store *store, panel *PanelHub) *AgentHub {
	return &AgentHub{store: store, panel: panel, conns: make(map[int64]*agentConn)}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Agent 为非浏览器客户端，且鉴权依赖 Authorization 头而非 Origin
	CheckOrigin: func(*http.Request) bool { return true },
}

// onlineInfo 面板事件与 API 输出共用的在线状态载荷。
type onlineInfo struct {
	ServerID     int64  `json:"server_id"`
	Hostname     string `json:"hostname"`
	AgentVersion string `json:"agent_version"`
}

// HandleAgentWS 处理 /ws/agent：Bearer token 认证 → 升级 → 注册 → 循环读。
func (h *AgentHub) HandleAgentWS(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		httpError(w, http.StatusUnauthorized, "缺少 Agent token")
		return
	}
	server, err := h.store.getServerByTokenHash(hashToken(token))
	if err != nil {
		httpError(w, http.StatusUnauthorized, "Agent token 无效或已重置")
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade 失败时它自己已写过 400/500 响应，无需重复处理
		log.Printf("[agent] 升级连接失败 server=%d: %v", server.ID, err)
		return
	}
	h.register(server.ID, conn)
}

// register 将连接放入在线表并启动读写泵；同服务器重复连入时踢掉旧连接。
func (h *AgentHub) register(serverID int64, conn *websocket.Conn) {
	ac := &agentConn{serverID: serverID, conn: conn}

	h.mu.Lock()
	if old := h.conns[serverID]; old != nil {
		// 旧连接由 close 触发其读泵退出；cleanup 会校验指针，不会误删本新连接
		old.conn.Close()
	}
	h.conns[serverID] = ac
	h.mu.Unlock()

	// 上线广播延迟到 hello：连接建立≠完成握手，此时还没有 hostname/版本可报
	go h.readPump(ac)
	go h.writePump(ac)
}

// unregister 只移除仍属于 ac 的表项，避免新旧连接交接竞态时误删新连接。
func (h *AgentHub) unregister(ac *agentConn) {
	h.mu.Lock()
	if cur, ok := h.conns[ac.serverID]; ok && cur == ac {
		delete(h.conns, ac.serverID)
	}
	h.mu.Unlock()

	ac.conn.Close()
	// 只有完成过握手的连接才广播下线，避免半开连接产生噪音事件
	if ac.announced {
		log.Printf("[agent] server=%d 离线", ac.serverID)
		h.panel.broadcastOffline(ac.serverID)
	} else {
		log.Printf("[agent] server=%d 未完成握手即断开", ac.serverID)
	}
}

// readPump 循环读取 Agent 消息；hello 与 heartbeat 都会刷新数据库里的最后心跳。
func (h *AgentHub) readPump(ac *agentConn) {
	defer h.unregister(ac)
	for {
		_ = ac.conn.SetReadDeadline(time.Now().Add(readTimeout))
		var msg agentMsg
		if err := ac.conn.ReadJSON(&msg); err != nil {
			return // 读超时、连接关闭或非法消息，一律按离线处理
		}
		switch msg.Type {
		case "hello":
			ac.hostname = msg.Hostname
			ac.version = msg.AgentVersion
			// 先落库再广播：保证事件到达观察者时，读库已能看到一致数据
			if err := h.store.agentSeen(ac.serverID, ac.hostname, ac.version); err != nil {
				log.Printf("[agent] 更新 server=%d 心跳失败: %v", ac.serverID, err)
			}
			if !ac.announced {
				ac.announced = true
				log.Printf("[agent] server=%d 上线 hostname=%q version=%q", ac.serverID, ac.hostname, ac.version)
				h.panel.broadcastOnline(onlineInfo{
					ServerID: ac.serverID, Hostname: ac.hostname, AgentVersion: ac.version,
				})
			}
		case "heartbeat":
			// 心跳静默处理，仅刷新最后在线时间
			if err := h.store.agentSeen(ac.serverID, ac.hostname, ac.version); err != nil {
				log.Printf("[agent] 更新 server=%d 心跳失败: %v", ac.serverID, err)
			}
		default:
			// M0 不存在其他指令；未知消息记日志但不掐线，协议向后兼容
			log.Printf("[agent] server=%d 发来未知消息 type=%q", ac.serverID, msg.Type)
		}
	}
}

// writePump 周期性下发应用层 ping，维持 Agent 侧的半开检测。
func (h *AgentHub) writePump(ac *agentConn) {
	ticker := time.NewTicker(agentPingInterval)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.Lock()
		_, alive := h.conns[ac.serverID]
		cur := h.conns[ac.serverID]
		h.mu.Unlock()
		if !alive || cur != ac {
			return
		}
		_ = ac.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := ac.conn.WriteJSON(map[string]any{"type": "ping", "ts": time.Now().Unix()}); err != nil {
			return
		}
	}
}

// onlineMap 返回 server_id → 在线信息快照，供 REST API 组装 online 字段。
func (h *AgentHub) onlineMap() map[int64]onlineInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[int64]onlineInfo, len(h.conns))
	for id, ac := range h.conns {
		out[id] = onlineInfo{ServerID: id, Hostname: ac.hostname, AgentVersion: ac.version}
	}
	return out
}

// kick 断开指定服务器的在线连接（删除服务器 / 重置 token 时调用）。
func (h *AgentHub) kick(serverID int64) {
	h.mu.Lock()
	ac := h.conns[serverID]
	h.mu.Unlock()
	if ac != nil {
		ac.conn.Close()
	}
}

// bearerToken 从 Authorization: Bearer <token> 中取出凭证。
func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) > len(prefix) && subtle.ConstantTimeCompare([]byte(auth[:len(prefix)]), []byte(prefix)) == 1 {
		return auth[len(prefix):]
	}
	return ""
}

// jsonWrite 统一 JSON 响应输出。
func jsonWrite(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[api] 写响应失败: %v", err)
	}
}

func httpError(w http.ResponseWriter, status int, msg string) {
	jsonWrite(w, status, map[string]string{"error": msg})
}
