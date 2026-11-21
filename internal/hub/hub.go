// Package hub 实现控制面：面板 API、Agent 长连接、在线状态。
package hub

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/SinGyoZ/TarShip/internal/store"
	"github.com/SinGyoZ/TarShip/internal/version"
)

var errNotHello = errors.New("首条消息不是 hello")

// Config 是 Hub 的运行配置。
type Config struct {
	Addr       string
	DataDir    string
	TrustProxy bool
}

// Hub 聚合 Hub 的全部组件。
type Hub struct {
	cfg      Config
	log      *slog.Logger
	ctx      context.Context // Hub 的生命周期；取消后所有 WebSocket 循环退出
	store    *store.Store
	auth     *authService
	presence *presence
	panel    *panelBroker
	wg       sync.WaitGroup // 跟踪 WebSocket handler，退出时等它们收尾

	// pendingTests 承接 local 存储器可写性测试的异步回报（testID → chan）
	pendingTests sync.Map
	// runProgress 缓存 Agent 上报的实时进度（runID → 已上传字节），不落库
	runProgress sync.Map
}

// New 组装 Hub，不监听端口。
func New(ctx context.Context, cfg Config, st *store.Store, log *slog.Logger) (*Hub, error) {
	auth, err := newAuthService(ctx, st, log, cfg.TrustProxy)
	if err != nil {
		return nil, err
	}
	return &Hub{
		cfg: cfg, log: log, ctx: ctx, store: st, auth: auth,
		presence: newPresence(), panel: newPanelBroker(),
	}, nil
}

// Run 打开数据库并运行 Hub，直到 ctx 取消。
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	h, err := New(ctx, cfg, st, log)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           h.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	// 上一次进程退出可能遗留 running 状态的运行，先清场再对外服务
	h.failStaleRuns()
	log.Info("Hub 已启动", "addr", cfg.Addr, "data", cfg.DataDir, "version", version.Version)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("Hub 正在退出")
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(sctx)
	h.wg.Wait() // WebSocket 连接已被 Hijack，Shutdown 不会等它们，这里等 handler 收尾后再关库
	return err
}

// Handler 返回完整的 HTTP 处理器。
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealth)

	mux.HandleFunc("GET /api/setup/status", h.auth.handleSetupStatus)
	mux.HandleFunc("POST /api/auth/setup", h.auth.handleSetup)
	mux.HandleFunc("POST /api/auth/login", h.auth.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", h.auth.handleLogout)
	mux.Handle("GET /api/auth/me", h.auth.require(http.HandlerFunc(h.auth.handleMe)))

	mux.Handle("GET /api/servers", h.auth.require(http.HandlerFunc(h.handleListServers)))
	mux.Handle("POST /api/servers", h.auth.require(http.HandlerFunc(h.handleCreateServer)))
	mux.Handle("DELETE /api/servers/{id}", h.auth.require(http.HandlerFunc(h.handleDeleteServer)))
	mux.Handle("POST /api/servers/{id}/reset-token", h.auth.require(http.HandlerFunc(h.handleResetToken)))
	mux.Handle("GET /api/dashboard", h.auth.require(http.HandlerFunc(h.handleDashboard)))

	// M1 备份闭环
	mux.Handle("GET /api/storages", h.auth.require(http.HandlerFunc(h.handleListStorages)))
	mux.Handle("POST /api/storages", h.auth.require(http.HandlerFunc(h.handleCreateStorage)))
	mux.Handle("PUT /api/storages/{id}", h.auth.require(http.HandlerFunc(h.handleUpdateStorage)))
	mux.Handle("DELETE /api/storages/{id}", h.auth.require(http.HandlerFunc(h.handleDeleteStorage)))
	mux.Handle("POST /api/storages/{id}/test", h.auth.require(http.HandlerFunc(h.handleTestStorage)))
	mux.Handle("GET /api/tasks", h.auth.require(http.HandlerFunc(h.handleListTasks)))
	mux.Handle("POST /api/tasks", h.auth.require(http.HandlerFunc(h.handleCreateTask)))
	mux.Handle("PUT /api/tasks/{id}", h.auth.require(http.HandlerFunc(h.handleUpdateTask)))
	mux.Handle("DELETE /api/tasks/{id}", h.auth.require(http.HandlerFunc(h.handleDeleteTask)))
	mux.Handle("POST /api/tasks/{id}/run", h.auth.require(http.HandlerFunc(h.handleRunTask)))
	mux.Handle("GET /api/runs", h.auth.require(http.HandlerFunc(h.handleListRuns)))
	mux.Handle("GET /api/backups", h.auth.require(http.HandlerFunc(h.handleListBackups)))

	mux.HandleFunc("GET /ws/agent", h.handleAgentWS)
	mux.Handle("GET /ws/panel", h.auth.require(http.HandlerFunc(h.handlePanelWS)))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
	})
	mux.Handle("/", spaHandler())
	return secureHeaders(csrfGuard(mux))
}

func (h *Hub) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
