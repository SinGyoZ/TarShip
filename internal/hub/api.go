package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/SinGyoZ/TarShip/internal/version"
)

// api 承载全部面板 REST 端点（方案 5.2 的 M0 子集）。
type api struct {
	store  *store
	auth   *authService
	agents *AgentHub
}

func newAPI(store *store, auth *authService, agents *AgentHub) *api {
	return &api{store: store, auth: auth, agents: agents}
}

// routes 注册所有 HTTP 路由；Go 1.22+ 方法路由按最长模式匹配，
// /api/* 全部经由 requireAuth 包裹鉴权（登录与初始化端点除外）。
func (a *api) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/auth/status", a.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/setup", a.handleSetup)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)

	mux.HandleFunc("GET /api/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("GET /api/dashboard", a.requireAuth(a.handleDashboard))
	mux.HandleFunc("GET /api/servers", a.requireAuth(a.handleListServers))
	mux.HandleFunc("POST /api/servers", a.requireAuth(a.handleCreateServer))
	mux.HandleFunc("DELETE /api/servers/{id}", a.requireAuth(a.handleDeleteServer))
	mux.HandleFunc("POST /api/servers/{id}/reset-token", a.requireAuth(a.handleResetToken))

	return mux
}

// requireAuth 保护需要登录态的端点，未认证统一返回 401。
func (a *api) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.auth.sessionUser(r); err != nil {
			httpError(w, http.StatusUnauthorized, "未登录或会话已过期")
			return
		}
		next(w, r)
	}
}

// ---- 鉴权端点 ----

func (a *api) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	// 该端点无需登录：登录页在进入前需要知道是"初始化"还是"登录"
	jsonWrite(w, http.StatusOK, map[string]any{
		"initialized": a.auth.initialized(),
		"version":     version.Version,
	})
}

func (a *api) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.auth.setup(strings.TrimSpace(req.Username), req.Password); err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	token, err := a.auth.issueToken(strings.TrimSpace(req.Username))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "签发会话失败")
		return
	}
	setSessionCookie(w, token)
	jsonWrite(w, http.StatusOK, map[string]any{"username": strings.TrimSpace(req.Username)})
}

func (a *api) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	username, err := a.auth.login(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		// 统一报错文案，不区分"用户不存在"与"密码错误"
		httpError(w, http.StatusUnauthorized, err.Error())
		return
	}
	token, err := a.auth.issueToken(username)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "签发会话失败")
		return
	}
	setSessionCookie(w, token)
	jsonWrite(w, http.StatusOK, map[string]any{"username": username})
}

func (a *api) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *api) handleMe(w http.ResponseWriter, r *http.Request) {
	username, _ := a.auth.sessionUser(r)
	jsonWrite(w, http.StatusOK, map[string]string{"username": username, "version": version.Version})
}

// ---- 服务器管理 ----

func (a *api) handleListServers(w http.ResponseWriter, r *http.Request) {
	servers, err := a.store.listServers()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "查询服务器列表失败")
		return
	}
	online := a.agents.onlineMap()
	out := make([]map[string]any, 0, len(servers))
	for _, s := range servers {
		row := map[string]any{
			"id":            s.ID,
			"name":          s.Name,
			"online":        false,
			"hostname":      s.Hostname,
			"agent_version": s.AgentVersion, // 离线时展示最后已知版本
			"tags":          s.Tags,
			"last_seen":     s.LastSeenAt,
			"created_at":    s.CreatedAt,
		}
		if info, ok := online[s.ID]; ok {
			row["online"] = true
			// 以 hello 上报值优先，避免离线残留的旧版本号误导
			row["hostname"] = info.Hostname
			row["agent_version"] = info.AgentVersion
		}
		out = append(out, row)
	}
	jsonWrite(w, http.StatusOK, out)
}

var hubURLPattern = regexp.MustCompile(`^https?://[^\s]+$`)

func (a *api) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Tags   string `json:"tags"`
		HubURL string `json:"hub_url"` // Agent 连接用的 Hub 外部地址，如 https://hub.example.com:8080
		Arch   string `json:"arch"`    // linux-amd64 / linux-arm64，默认 amd64
	}
	if err := decodeJSON(w, r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := sanitizeName(req.Name)
	if name == "" {
		httpError(w, http.StatusBadRequest, "服务器名称不能为空")
		return
	}
	hubURL := strings.TrimRight(strings.TrimSpace(req.HubURL), "/")
	if !hubURLPattern.MatchString(hubURL) {
		httpError(w, http.StatusBadRequest, "Hub 地址需以 http:// 或 https:// 开头")
		return
	}
	arch := strings.TrimSpace(req.Arch)
	if arch == "" {
		arch = "amd64"
	}
	if arch != "amd64" && arch != "arm64" {
		httpError(w, http.StatusBadRequest, "arch 仅支持 amd64 / arm64")
		return
	}
	plain, hash, err := newServerToken()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	srv, err := a.store.createServer(name, hash, strings.TrimSpace(req.Tags))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "创建服务器失败")
		return
	}
	log.Printf("[api] 创建服务器 id=%d name=%q", srv.ID, srv.Name)
	jsonWrite(w, http.StatusCreated, map[string]any{
		"id":              srv.ID,
		"name":            srv.Name,
		"token":           plain, // 明文 token 仅此一次返回，库中只存 SHA-256
		"install_command": installCommand(hubURL, plain, arch),
	})
}

func (a *api) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpError(w, http.StatusBadRequest, "无效的服务器 ID")
		return
	}
	if err := a.store.deleteServer(id); err != nil {
		if errors.Is(err, errNotFound) {
			httpError(w, http.StatusNotFound, "服务器不存在")
			return
		}
		httpError(w, http.StatusInternalServerError, "删除服务器失败")
		return
	}
	// 先断连接再广播，面板状态与连接表保持一致
	a.agents.kick(id)
	a.agents.panel.broadcastRemoved(id)
	log.Printf("[api] 删除服务器 id=%d", id)
	jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *api) handleResetToken(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpError(w, http.StatusBadRequest, "无效的服务器 ID")
		return
	}
	var req struct {
		HubURL string `json:"hub_url"`
		Arch   string `json:"arch"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	hubURL := strings.TrimRight(strings.TrimSpace(req.HubURL), "/")
	if !hubURLPattern.MatchString(hubURL) {
		httpError(w, http.StatusBadRequest, "Hub 地址需以 http:// 或 https:// 开头")
		return
	}
	arch := strings.TrimSpace(req.Arch)
	if arch == "" {
		arch = "amd64"
	}
	plain, err := a.store.resetServerToken(id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			httpError(w, http.StatusNotFound, "服务器不存在")
			return
		}
		httpError(w, http.StatusInternalServerError, "重置 token 失败")
		return
	}
	// 旧 token 立即失效：踢掉现连接，等待 Agent 侧用新 token 重连
	a.agents.kick(id)
	log.Printf("[api] 重置服务器 token id=%d", id)
	jsonWrite(w, http.StatusOK, map[string]any{
		"token":           plain,
		"install_command": installCommand(hubURL, plain, arch),
	})
}

// ---- 仪表盘 ----

func (a *api) handleDashboard(w http.ResponseWriter, r *http.Request) {
	servers, err := a.store.listServers()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "查询统计失败")
		return
	}
	online := a.agents.onlineMap()
	total, onlineCount, outdated := len(servers), 0, 0
	for _, s := range servers {
		if info, ok := online[s.ID]; ok {
			onlineCount++
			if isOutdated(info.AgentVersion, version.Version) {
				outdated++
			}
		}
	}
	jsonWrite(w, http.StatusOK, map[string]any{
		"version": version.Version,
		"servers": map[string]int{
			"total":    total,
			"online":   onlineCount,
			"offline":  total - onlineCount,
			"outdated": outdated,
		},
	})
}

// ---- 工具函数 ----

// isOutdated 用语义化版本比较判断 Agent 是否落后于 Hub。
// 任一方非 semver（如 dev / 本地构建号）时不计入，避免字符串不等导致误报。
func isOutdated(agentVersion, hubVersion string) bool {
	if !semver.IsValid(agentVersion) || !semver.IsValid(hubVersion) {
		return false
	}
	return semver.Compare(agentVersion, hubVersion) < 0
}

// installCommand 按方案 11.2 生成 Agent 一键安装命令（systemd 裸机部署）。
// 三处细节缺一不可：
//  1. 先下载到 /tmp 再 mv——直接 -o 覆盖正在运行的二进制会触发 ETXTBSY 写入保护，
//     rename 原子替换目录项则不受影响（升级重装场景 Agent 仍在运行）；
//  2. 结尾 daemon-reload + restart——enable --now 对已运行服务不会重启；
//  3. enable 保持幂等，全新安装与升级重装同一份命令。
func installCommand(hubURL, token, arch string) string {
	const tmpl = `curl -fsSL https://github.com/SinGyoZ/TarShip/releases/latest/download/tarship-linux-%s -o /tmp/tarship.new
chmod +x /tmp/tarship.new
mv /tmp/tarship.new /usr/local/bin/tarship
cat > /etc/systemd/system/tarship-agent.service <<'EOF'
[Unit]
Description=TarShip Agent
After=network-online.target
[Service]
ExecStart=/usr/local/bin/tarship agent --server %s --token %s
Restart=always
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable tarship-agent
systemctl restart tarship-agent`
	return fmt.Sprintf(tmpl, arch, hubURL, token)
}

// decodeJSON 解析请求体；限制 32KB 防异常超大请求。
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("请求体不是合法 JSON")
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
