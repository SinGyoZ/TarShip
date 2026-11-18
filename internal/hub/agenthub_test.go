package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestHub 搭建完整的 Hub 链路（不含前端），返回各组件与 httptest 地址。
func newTestHub(t *testing.T) (*store, *authService, *AgentHub, *PanelHub, *httptest.Server) {
	t.Helper()
	st := newTestStore(t)
	authSvc, err := newAuth(st, t.TempDir())
	if err != nil {
		t.Fatalf("初始化 auth: %v", err)
	}
	if err := authSvc.setup("admin", "s3cret-pass"); err != nil {
		t.Fatalf("初始化账号: %v", err)
	}
	panel := newPanelHub(authSvc)
	agents := newAgentHub(st, panel)
	panel.bindSnapshot(agents.onlineMap)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/agent", agents.HandleAgentWS)
	mux.HandleFunc("/ws/panel", panel.HandlePanelWS)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return st, authSvc, agents, panel, ts
}

func wsURL(httpURL, path string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + path
}

func TestAgentOnlineOffline(t *testing.T) {
	st, _, agents, _, ts := newTestHub(t)
	plain, hash, _ := newServerToken()
	srv, err := st.createServer("vps-01", hash, "")
	if err != nil {
		t.Fatalf("创建服务器: %v", err)
	}

	// 错误 token 必须被拒绝
	bad := wsDial403(t, wsURL(ts.URL, "/ws/agent"), "tsk_wrong-token-0000000000000000")
	if !bad {
		t.Fatal("错误 token 竟然连接成功")
	}

	// 正确 token 连接并 hello
	conn := wsDial(t, wsURL(ts.URL, "/ws/agent"), plain)
	defer conn.Close()
	mustSendJSON(t, conn, map[string]any{"type": "hello", "agent_version": "0.1.0", "hostname": "real-host"})

	waitFor(t, 3*time.Second, func() bool {
		m := agents.onlineMap()
		info, ok := m[srv.ID]
		return ok && info.Hostname == "real-host"
	})
	// hello 上报应写入数据库
	got, _ := st.getServer(srv.ID)
	if got.Hostname != "real-host" || got.AgentVersion != "0.1.0" || got.LastSeenAt == "" {
		t.Fatalf("hello 信息未入库: %+v", got)
	}

	// 心跳后再断开，在线表必须清空
	mustSendJSON(t, conn, map[string]any{"type": "heartbeat", "ts": time.Now().Unix()})
	_ = conn.Close()
	waitFor(t, 3*time.Second, func() bool {
		_, ok := agents.onlineMap()[srv.ID]
		return !ok
	})
}

// wsDial403 返回 true 表示带该 token 连接被拒绝（预期 401/403）。
func wsDial403(t *testing.T, url, token string) bool {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err == nil {
		conn.Close()
		return false
	}
	if resp != nil {
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusUnauthorized
	}
	return false
}

func TestPanelBroadcast(t *testing.T) {
	st, authSvc, _, _, ts := newTestHub(t)
	plain, hash, _ := newServerToken()
	srv, err := st.createServer("vps-02", hash, "")
	if err != nil {
		t.Fatalf("创建服务器: %v", err)
	}

	// 面板以会话 Cookie 建立 WS 订阅
	token, _ := authSvc.issueToken("admin")
	header := http.Header{}
	header.Set("Cookie", cookieName+"="+token)
	panelConn, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL, "/ws/panel"), header)
	if err != nil {
		t.Fatalf("面板连接: %v", err)
	}
	defer panelConn.Close()

	// gorilla/websocket 的连接读失败一次后不可重读，
	// 因此用单个 goroutine 持续读事件并写入 channel，断言在主协程完成
	eventsCh := make(chan map[string]any, 16)
	go func() {
		defer close(eventsCh)
		for {
			_ = panelConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, raw, err := panelConn.ReadMessage()
			if err != nil {
				return
			}
			var ev map[string]any
			if json.Unmarshal(raw, &ev) == nil {
				eventsCh <- ev
			}
		}
	}()

	// 未认证面板连接必须被拒绝
	if ok := panelDialUnauthorized(t, ts.URL); !ok {
		t.Fatal("未登录面板连接竟被放行")
	}

	// 面板连接注册完成的确定性同步点：收到 panel_hello 后再触发 Agent 上线，
	// 消除"注册前广播丢失"的调度竞态
	if expectEvent(t, eventsCh, "panel_hello", 3*time.Second) == nil {
		t.Fatal("面板未收到注册回执 panel_hello")
	}

	// Agent 上线 → 面板应收到 server_online
	agentConn := wsDial(t, wsURL(ts.URL, "/ws/agent"), plain)
	defer agentConn.Close()
	mustSendJSON(t, agentConn, map[string]any{"type": "hello", "agent_version": "0.1.0", "hostname": "h2"})

	ev := expectEvent(t, eventsCh, "server_online", 3*time.Second)
	if ev == nil {
		t.Fatal("面板未收到 server_online 事件")
	}
	if data, _ := ev["data"].(map[string]any); data == nil || data["hostname"] != "h2" {
		t.Fatalf("server_online 载荷异常: %v", ev)
	}

	// Agent 断开 → 面板应收到 server_offline
	_ = agentConn.Close()
	if expectEvent(t, eventsCh, "server_offline", 3*time.Second) == nil {
		t.Fatal("面板未收到 server_offline 事件")
	}
	_ = srv.ID
}

// expectEvent 从事件 channel 中等待目标类型事件；超时返回 nil。
func expectEvent(t *testing.T, ch <-chan map[string]any, typ string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if ev["type"] == typ {
				return ev
			}
		case <-deadline:
			return nil
		}
	}
}

func panelDialUnauthorized(t *testing.T, httpURL string) bool {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL(httpURL, "/ws/panel"), nil)
	if err == nil {
		conn.Close()
		return false
	}
	if resp != nil {
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusUnauthorized
	}
	return false
}
