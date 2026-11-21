package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/SinGyoZ/TarShip/internal/proto"
	"github.com/SinGyoZ/TarShip/internal/version"
)

// lastSeenFlushInterval 限制心跳触发的数据库写入频率。
const lastSeenFlushInterval = 60 * time.Second

func (h *Hub) handleAgentWS(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	srv, ok, err := h.store.GetServerByTokenHash(r.Context(), hashToken(token))
	if err != nil {
		h.log.Error("查询服务器失败", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	h.wg.Add(1)
	defer h.wg.Done()
	defer c.CloseNow()
	c.SetReadLimit(proto.MaxMessageBytes)

	hello, err := readHello(h.ctx, c)
	if err != nil {
		c.Close(proto.CloseBadHello, "首条消息必须是合法的 hello")
		return
	}
	if hello.ProtocolVersion != proto.Version {
		h.log.Warn("Agent 协议版本不兼容", "server", srv.Name, "agent_protocol", hello.ProtocolVersion, "hub_protocol", proto.Version)
		c.Close(proto.CloseProtocolMismatch, "协议版本不兼容，请升级 Agent 或 Hub")
		return
	}

	now := time.Now()
	sess := &agentSession{
		serverID: srv.ID, conn: c, hostname: cleanText(hello.Hostname, 255),
		version: cleanText(hello.AgentVersion, 64), lastSeen: now, lastFlush: now,
	}
	if old := h.presence.register(sess); old != nil {
		go old.conn.Close(proto.CloseReplaced, "同一 token 有新的连接")
	}
	h.dbCall(func(ctx context.Context) error {
		return h.store.TouchServer(ctx, srv.ID, sess.hostname, sess.version, now)
	})
	h.publishStatus(srv.ID, true, sess.hostname, sess.version, now)
	h.log.Info("Agent 已上线", "server", srv.Name, "hostname", sess.hostname, "version", sess.version)

	welcome := proto.Welcome{
		Type: proto.TypeWelcome, ServerID: srv.ID, Name: srv.Name, HubVersion: version.Version,
		ProtocolVersion: proto.Version, HeartbeatIntervalSec: int(proto.HeartbeatInterval / time.Second),
	}
	reason := h.serveAgent(sess, welcome)

	if h.presence.unregister(sess) {
		end := time.Now()
		h.dbCall(func(ctx context.Context) error { return h.store.UpdateLastSeen(ctx, srv.ID, end) })
		h.publishStatus(srv.ID, false, "", "", end)
		h.log.Info("Agent 已离线", "server", srv.Name, "reason", reason)
	}
	c.Close(websocket.StatusGoingAway, "")
}

// serveAgent 处理已注册连接的消息循环，返回结束原因。
func (h *Hub) serveAgent(sess *agentSession, welcome proto.Welcome) error {
	if err := writeJSONMsg(h.ctx, sess.conn, welcome); err != nil {
		return err
	}
	for {
		rctx, cancel := context.WithTimeout(h.ctx, proto.OfflineTimeout)
		_, data, err := sess.conn.Read(rctx)
		cancel()
		if err != nil {
			return err
		}
		now := time.Now()
		h.presence.touch(sess, now)

		typ, err := proto.PeekType(data)
		if err != nil {
			continue
		}
		switch typ {
		case proto.TypeHeartbeat:
			if err := writeJSONMsg(h.ctx, sess.conn, proto.HeartbeatAck{Type: proto.TypeHeartbeatAck}); err != nil {
				return err
			}
			if now.Sub(sess.lastFlush) >= lastSeenFlushInterval {
				sess.lastFlush = now
				h.dbCall(func(ctx context.Context) error { return h.store.UpdateLastSeen(ctx, sess.serverID, now) })
			}
		default:
			h.log.Debug("忽略未知消息类型", "type", typ)
		}
	}
}

func readHello(ctx context.Context, c *websocket.Conn) (proto.Hello, error) {
	rctx, cancel := context.WithTimeout(ctx, proto.HelloTimeout)
	defer cancel()
	_, data, err := c.Read(rctx)
	if err != nil {
		return proto.Hello{}, err
	}
	var hello proto.Hello
	if err := json.Unmarshal(data, &hello); err != nil {
		return proto.Hello{}, err
	}
	if hello.Type != proto.TypeHello {
		return proto.Hello{}, errNotHello
	}
	return hello, nil
}

func writeJSONMsg(ctx context.Context, c *websocket.Conn, v any) error {
	wctx, cancel := context.WithTimeout(ctx, proto.WriteTimeout)
	defer cancel()
	return wsjson.Write(wctx, c, v)
}

// dbCall 在独立的短超时上下文里执行数据库操作：Hub 关闭时 h.ctx 已取消，
// 但收尾写入仍应尽量完成；失败只记日志，不影响连接处理。
func (h *Hub) dbCall(fn func(ctx context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		h.log.Warn("数据库写入失败", "error", err)
	}
}

// cleanText 去掉控制字符并截断，防止 Agent 上报的内容污染日志和面板。
func cleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}
