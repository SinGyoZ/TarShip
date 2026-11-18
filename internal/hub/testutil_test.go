package hub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 测试侧小工具，避免每个用例重复样板代码。

func mustNewRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatalf("构造请求: %v", err)
	}
	return req
}

func mustDo(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("执行请求: %v", err)
	}
	return resp
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// wsDial 以 Agent 身份连接测试服务器。
func wsDial(t *testing.T, url, token string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("连接 %s: %v", url, err)
	}
	return conn
}

func mustSendJSON(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	if err := conn.WriteJSON(v); err != nil {
		t.Fatalf("发送消息: %v", err)
	}
}

// waitFor 轮询直到条件满足或超时，用于异步事件断言。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

func jsonBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("解析响应 %q: %v", string(raw), err)
	}
	return m
}
