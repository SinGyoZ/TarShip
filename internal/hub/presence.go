package hub

import (
	"sync"
	"time"

	"github.com/coder/websocket"
)

// agentSession 是一条已通过 hello 的 Agent 连接。
type agentSession struct {
	serverID  int64
	conn      *websocket.Conn
	hostname  string
	version   string
	lastSeen  time.Time // 由 presence.mu 保护
	lastFlush time.Time // 仅所属 handler 的 goroutine 访问
}

// presence 在内存里维护在线状态：在线与否以它为准，数据库只在节流后记录 last_seen_at。
type presence struct {
	mu       sync.RWMutex
	sessions map[int64]*agentSession
}

func newPresence() *presence {
	return &presence{sessions: map[int64]*agentSession{}}
}

// register 登记新连接；同一服务器已有旧连接时返回它，由调用方负责关闭。
func (p *presence) register(s *agentSession) (old *agentSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	old = p.sessions[s.serverID]
	p.sessions[s.serverID] = s
	return old
}

// unregister 仅在 s 仍是当前连接时移除，返回是否移除。
// 返回 false 说明它已被新连接顶替，此时不应再广播离线。
func (p *presence) unregister(s *agentSession) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions[s.serverID] != s {
		return false
	}
	delete(p.sessions, s.serverID)
	return true
}

func (p *presence) touch(s *agentSession, t time.Time) {
	p.mu.Lock()
	s.lastSeen = t
	p.mu.Unlock()
}

func (p *presence) status(id int64) (online bool, lastSeen time.Time) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s := p.sessions[id]
	if s == nil {
		return false, time.Time{}
	}
	return true, s.lastSeen
}

// agentVersion 返回在线会话 hello 上报的 Agent 版本；未在线时返回空串。
// version 在 register 时赋值后不再变化，此处只读。
func (p *presence) agentVersion(id int64) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if s := p.sessions[id]; s != nil {
		return s.version
	}
	return ""
}

// kick 关闭某台服务器当前的连接（服务器被删除、token 被重置时使用）。
func (p *presence) kick(id int64, code websocket.StatusCode, reason string) {
	p.mu.RLock()
	s := p.sessions[id]
	p.mu.RUnlock()
	if s != nil {
		go s.conn.Close(code, reason) // Close 会等对端握手，不能阻塞调用方
	}
}
