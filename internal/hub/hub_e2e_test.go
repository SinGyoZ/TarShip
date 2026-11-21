package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/SinGyoZ/TarShip/internal/agent"
	"github.com/SinGyoZ/TarShip/internal/store"
)

type testEnv struct {
	t      *testing.T
	ctx    context.Context
	hub    *Hub
	ts     *httptest.Server
	client *http.Client
	jar    *cookiejar.Jar
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(ctx, Config{}, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h.Handler())
	jar, _ := cookiejar.New(nil)
	t.Cleanup(func() {
		cancel()
		h.wg.Wait()
		ts.Close()
		st.Close()
	})
	return &testEnv{t: t, ctx: ctx, hub: h, ts: ts, jar: jar, client: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// call 发送 JSON 请求并断言状态码，out 非空时解析响应。
func (e *testEnv) call(method, path string, body any, want int, out any) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		e.t.Fatalf("%s %s = %d; want %d (%s)", method, path, resp.StatusCode, want, b)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			e.t.Fatal(err)
		}
	}
}

func (e *testEnv) login() {
	e.t.Helper()
	e.call("POST", "/api/auth/setup", map[string]string{
		"setup_token": e.hub.auth.currentSetupToken(), "username": "admin", "password": "correct-horse",
	}, 200, nil)
}

func (e *testEnv) createServer(name string) (id int64, token string) {
	e.t.Helper()
	var out struct {
		Server serverDTO `json:"server"`
		Token  string    `json:"token"`
	}
	e.call("POST", "/api/servers", map[string]any{"name": name}, 201, &out)
	return out.Server.ID, out.Token
}

func (e *testEnv) wsURL(path string) string {
	return "ws" + strings.TrimPrefix(e.ts.URL, "http") + path
}

func (e *testEnv) dialPanel() *websocket.Conn {
	e.t.Helper()
	u, _ := url.Parse(e.ts.URL)
	var parts []string
	for _, c := range e.jar.Cookies(u) {
		parts = append(parts, c.Name+"="+c.Value)
	}
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, e.wsURL("/ws/panel"), &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": []string{strings.Join(parts, "; ")}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { c.CloseNow() })
	return c
}

// waitEvent 读取面板事件，直到 match 返回 true。
func (e *testEnv) waitEvent(c *websocket.Conn, match func(map[string]any) bool) map[string]any {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			e.t.Fatalf("等待面板事件失败: %v", err)
		}
		var ev map[string]any
		if err := json.Unmarshal(data, &ev); err != nil {
			e.t.Fatal(err)
		}
		if match(ev) {
			return ev
		}
	}
}

func statusIs(id int64, online bool) func(map[string]any) bool {
	return func(ev map[string]any) bool {
		return ev["type"] == "server_status" && int64(ev["server_id"].(float64)) == id && ev["online"] == online
	}
}

func (e *testEnv) startAgent(token string) (stop func(), done <-chan error) {
	e.t.Helper()
	a, err := agent.New(agent.Config{Server: e.wsURL(""), Token: token, AllowInsecureWS: true},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		e.t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(e.ctx)
	ch := make(chan error, 1)
	go func() { ch <- a.Run(ctx) }()
	e.t.Cleanup(cancel)
	return cancel, ch
}

func TestSetupAndLoginFlow(t *testing.T) {
	e := newEnv(t)

	var st struct{ Initialized bool }
	e.call("GET", "/api/setup/status", nil, 200, &st)
	if st.Initialized {
		t.Fatal("全新实例不应已初始化")
	}
	e.call("GET", "/api/servers", nil, 401, nil)

	e.call("POST", "/api/auth/setup", map[string]string{
		"setup_token": "wrong", "username": "admin", "password": "correct-horse"}, 403, nil)
	e.call("POST", "/api/auth/setup", map[string]string{
		"setup_token": e.hub.auth.currentSetupToken(), "username": "ab", "password": "correct-horse"}, 400, nil)
	e.call("POST", "/api/auth/setup", map[string]string{
		"setup_token": e.hub.auth.currentSetupToken(), "username": "admin", "password": "short"}, 400, nil)

	e.login()
	e.call("GET", "/api/setup/status", nil, 200, &st)
	if !st.Initialized {
		t.Fatal("创建管理员后应为已初始化")
	}
	e.call("GET", "/api/auth/me", nil, 200, nil)
	e.call("POST", "/api/auth/setup", map[string]string{
		"setup_token": "x", "username": "other", "password": "correct-horse"}, 409, nil)

	e.call("POST", "/api/auth/logout", nil, 204, nil)
	e.call("GET", "/api/auth/me", nil, 401, nil)

	e.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "nope-nope"}, 401, nil)
	e.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "correct-horse"}, 200, nil)
	e.call("GET", "/api/auth/me", nil, 200, nil)
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t)
	e.login()
	for i := 0; i < maxLoginFails; i++ {
		e.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "wrong-pass"}, 401, nil)
	}
	e.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "correct-horse"}, 429, nil)
}

func TestCrossOriginPostRejected(t *testing.T) {
	e := newEnv(t)
	body, _ := json.Marshal(map[string]string{"username": "a", "password": "b"})
	req, _ := http.NewRequest("POST", e.ts.URL+"/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Origin", "http://evil.example")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("跨站 POST 应返回 403, got %d", resp.StatusCode)
	}
}

func TestAgentOnlineOfflineFlow(t *testing.T) {
	e := newEnv(t)
	e.login()
	id, token := e.createServer("vps-01")
	panel := e.dialPanel()

	snap := e.waitEvent(panel, func(ev map[string]any) bool { return ev["type"] == "snapshot" })
	if list := snap["servers"].([]any); len(list) != 1 || list[0].(map[string]any)["online"] != false {
		t.Fatalf("快照应含一台离线服务器: %v", snap)
	}

	stop, done := e.startAgent(token)
	ev := e.waitEvent(panel, statusIs(id, true))
	if ev["hostname"] == "" || ev["agent_version"] == "" {
		t.Fatalf("上线事件应带主机名和版本: %v", ev)
	}

	var list []serverDTO
	e.call("GET", "/api/servers", nil, 200, &list)
	if len(list) != 1 || !list[0].Online || list[0].LastSeenAt == nil || list[0].Hostname == "" {
		t.Fatalf("在线服务器列表不对: %+v", list)
	}

	stop()
	e.waitEvent(panel, statusIs(id, false))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("正常退出不应返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Agent 未在取消后退出")
	}
	e.call("GET", "/api/servers", nil, 200, &list)
	if list[0].Online || list[0].LastSeenAt == nil {
		t.Fatalf("离线后应保留最后在线时间: %+v", list[0])
	}
}

func TestAgentRejectedWithBadToken(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, e.wsURL("/ws/agent"), &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer tsa_wrong"}},
	})
	if err == nil || resp == nil || resp.StatusCode != 401 {
		t.Fatalf("错误 token 应得到 401: resp=%v err=%v", resp, err)
	}
}

func TestSameTokenReplacesOldConnection(t *testing.T) {
	e := newEnv(t)
	e.login()
	id, token := e.createServer("vps-01")
	panel := e.dialPanel()

	_, done1 := e.startAgent(token)
	e.waitEvent(panel, statusIs(id, true))
	_, _ = e.startAgent(token)

	select {
	case err := <-done1:
		if err == nil || !strings.Contains(err.Error(), "顶替") {
			t.Fatalf("被顶替的 Agent 应以致命错误退出, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("旧 Agent 未退出")
	}
	var list []serverDTO
	e.call("GET", "/api/servers", nil, 200, &list)
	if !list[0].Online {
		t.Fatal("新连接应保持在线")
	}
}

func TestDeleteServerKicksAgentAndResetInvalidatesToken(t *testing.T) {
	e := newEnv(t)
	e.login()
	id, token := e.createServer("vps-01")
	panel := e.dialPanel()
	e.startAgent(token)
	e.waitEvent(panel, statusIs(id, true))

	var reset struct{ Token string }
	e.call("POST", "/api/servers/"+itoa(id)+"/reset-token", nil, 200, &reset)
	if reset.Token == "" || reset.Token == token {
		t.Fatal("应返回新的 token")
	}
	e.waitEvent(panel, statusIs(id, false)) // 旧 token 的连接被踢

	e.call("DELETE", "/api/servers/"+itoa(id), nil, 204, nil)
	e.call("DELETE", "/api/servers/"+itoa(id), nil, 404, nil)
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestHealthz(t *testing.T) {
	e := newEnv(t)
	e.call("GET", "/healthz", nil, 200, nil)
}
