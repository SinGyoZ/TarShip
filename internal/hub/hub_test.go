package hub

import (
	"net/http/httptest"
	"testing"

	"github.com/SinGyoZ/TarShip/internal/db"
)

func newTestStore(t *testing.T) *store {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开测试库: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return &store{db: d}
}

func TestServerTokenRoundTrip(t *testing.T) {
	plain, hash, err := newServerToken()
	if err != nil {
		t.Fatalf("生成 token: %v", err)
	}
	if len(plain) < 32 || hashToken(plain) != hash {
		t.Fatalf("token 摘要不匹配: plain=%q hash=%q", plain, hash)
	}
	// 两次生成的 token 必须不同
	plain2, _, _ := newServerToken()
	if plain == plain2 {
		t.Fatal("随机 token 出现重复")
	}
}

func TestServerCRUDAndAgentSeen(t *testing.T) {
	st := newTestStore(t)
	_, hash, _ := newServerToken()
	srv, err := st.createServer("vps-01", hash, "prod,web")
	if err != nil {
		t.Fatalf("创建服务器: %v", err)
	}
	if srv.Name != "vps-01" || srv.Tags != "prod,web" {
		t.Fatalf("字段不符: %+v", srv)
	}
	// 按 token 摘要查（Agent 建连认证路径）
	got, err := st.getServerByTokenHash(hash)
	if err != nil || got.ID != srv.ID {
		t.Fatalf("按摘要查询失败: %v %+v", err, got)
	}
	// 心跳更新 hostname / 版本 / 时间
	if err := st.agentSeen(srv.ID, "real-host", "9.9.9"); err != nil {
		t.Fatalf("更新心跳: %v", err)
	}
	got, _ = st.getServer(srv.ID)
	if got.Hostname != "real-host" || got.AgentVersion != "9.9.9" || got.LastSeenAt == "" {
		t.Fatalf("心跳未生效: %+v", got)
	}
	// 删除后 token 查不到，再删报 not found
	if err := st.deleteServer(srv.ID); err != nil {
		t.Fatalf("删除服务器: %v", err)
	}
	if _, err := st.getServerByTokenHash(hash); err != errNotFound {
		t.Fatalf("删除后仍可认证，存在安全隐患: %v", err)
	}
	if err := st.deleteServer(srv.ID); err != errNotFound {
		t.Fatalf("重复删除应报 not found: %v", err)
	}
}

func TestResetToken(t *testing.T) {
	st := newTestStore(t)
	_, hash, _ := newServerToken()
	srv, _ := st.createServer("vps", hash, "")
	plain, err := st.resetServerToken(srv.ID)
	if err != nil {
		t.Fatalf("重置 token: %v", err)
	}
	// 新明文的摘要必须已入库，旧 token 立即失效
	if _, err := st.getServerByTokenHash(hashToken(plain)); err != nil {
		t.Fatalf("新 token 无法认证: %v", err)
	}
	if _, err := st.getServerByTokenHash(hash); err != errNotFound {
		t.Fatalf("旧 token 仍可认证: %v", err)
	}
}

func TestSetupLoginFlow(t *testing.T) {
	st := newTestStore(t)
	dir := t.TempDir()
	a, err := newAuth(st, dir)
	if err != nil {
		t.Fatalf("初始化 auth: %v", err)
	}
	if a.initialized() {
		t.Fatal("全新库不应处于已初始化状态")
	}
	// 弱密码应被拒绝
	if err := a.setup("admin", "123"); err == nil {
		t.Fatal("弱密码未被拒绝")
	}
	if err := a.setup("admin", "s3cret-pass"); err != nil {
		t.Fatalf("首启初始化: %v", err)
	}
	// 第二次 setup 必须拒绝
	if err := a.setup("hacker", "pwn-pass-123"); err == nil {
		t.Fatal("重复初始化未被拒绝")
	}
	// 错误密码拒绝、正确密码放行
	if _, err := a.login("admin", "wrong-pass"); err == nil {
		t.Fatal("错误密码通过校验")
	}
	if _, err := a.login("admin", "s3cret-pass"); err != nil {
		t.Fatalf("正确密码被拒绝: %v", err)
	}
	// JWT 往返
	token, err := a.issueToken("admin")
	if err != nil {
		t.Fatalf("签发 JWT: %v", err)
	}
	user, err := a.parseToken(token)
	if err != nil || user != "admin" {
		t.Fatalf("解析 JWT: user=%q err=%v", user, err)
	}
	// 篡改的 token 必须被拒绝
	if _, err := a.parseToken(token + "x"); err == nil {
		t.Fatal("篡改 token 通过校验")
	}
}

func TestSanitizeName(t *testing.T) {
	if sanitizeName("  a b  ") != "a b" {
		t.Fatalf("未去除空白: %q", sanitizeName("  a b  "))
	}
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	if len(sanitizeName(string(long))) != 100 {
		t.Fatal("超长名称未截断")
	}
}

func TestIsOutdated(t *testing.T) {
	cases := []struct {
		agent, hub string
		want       bool
	}{
		{"v0.1.1", "v0.1.1", false}, // 同版本
		{"v0.1.1", "v0.1.2", true},  // Agent 落后一个 patch
		{"v0.2.0", "v0.1.1", false}, // Agent 版本更新（不计落后）
		{"v0.1.1", "dev", false},    // Hub 为开发版，无法比较不计入
		{"dev", "v0.1.1", false},    // Agent 为开发版，无法比较不计入
		{"0.1.1", "v0.1.2", false},  // 无 v 前缀视为非 semver
	}
	for _, c := range cases {
		if got := isOutdated(c.agent, c.hub); got != c.want {
			t.Fatalf("isOutdated(%q, %q) = %v, 期望 %v", c.agent, c.hub, got, c.want)
		}
	}
}

func TestInstallCommand(t *testing.T) {
	cmd := installCommand("https://hub.example.com:8080", "tsk_abc", "amd64")
	// 安装命令必须包含关键元素：下载、systemd 单元、server 与 token 参数；
	// 下载必须落 /tmp 再 mv（运行中的二进制不可直接覆盖，ETXTBSY）；
	// 结尾必须是 daemon-reload + enable + restart，保证升级重装时新二进制生效
	for _, want := range []string{
		"tarship-linux-amd64", "--server https://hub.example.com:8080",
		"--token tsk_abc", "-o /tmp/tarship.new", "mv /tmp/tarship.new /usr/local/bin/tarship",
		"systemctl daemon-reload", "systemctl enable tarship-agent", "systemctl restart tarship-agent",
	} {
		if !contains(cmd, want) {
			t.Fatalf("安装命令缺少 %q:\n%s", want, cmd)
		}
	}
}

func TestAPILoginCookie(t *testing.T) {
	st := newTestStore(t)
	a, _ := newAuth(st, t.TempDir())
	_ = a.setup("admin", "s3cret-pass")
	ap := newAPI(st, a, newAgentHub(st, newPanelHub(a)))
	ts := httptest.NewServer(ap.routes())
	defer ts.Close()

	// 未登录访问受保护端点必须 401
	req := mustNewRequest(t, "GET", ts.URL+"/api/me", "")
	resp := mustDo(t, req)
	if resp.StatusCode != 401 {
		t.Fatalf("未登录访问 /api/me 期望 401, got %d", resp.StatusCode)
	}
	// 登录后的 Cookie 必须能访问受保护端点
	req = mustNewRequest(t, "POST", ts.URL+"/api/auth/login", `{"username":"admin","password":"s3cret-pass"}`)
	resp = mustDo(t, req)
	if resp.StatusCode != 200 {
		t.Fatalf("登录失败: %d", resp.StatusCode)
	}
	var cookie string
	for _, c := range resp.Cookies() {
		if c.Name == cookieName {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("登录响应缺少会话 Cookie")
	}
	req = mustNewRequest(t, "GET", ts.URL+"/api/me", "")
	req.Header.Set("Cookie", cookieName+"="+cookie)
	resp = mustDo(t, req)
	if resp.StatusCode != 200 {
		t.Fatalf("登录后访问 /api/me 期望 200, got %d", resp.StatusCode)
	}
}
