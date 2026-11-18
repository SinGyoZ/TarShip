// Package hub 为 TarShip 控制面：REST API、Agent/面板 WebSocket、SQLite 与内嵌面板。
package hub

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SinGyoZ/TarShip/internal/db"
	"github.com/SinGyoZ/TarShip/internal/version"
	webui "github.com/SinGyoZ/TarShip/web"
)

// Run 解析 hub 子命令参数并启动控制面（M0 验收项：子命令参数齐全）。
func Run(args []string) {
	fs := flag.NewFlagSet("hub", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "HTTP 监听地址")
	data := fs.String("data", "/data", "数据目录（SQLite、JWT 密钥等；裸机部署请指定可写目录）")
	_ = fs.Parse(args)

	if err := run(*addr, *data); err != nil {
		log.Fatalf("[hub] 启动失败: %v", err)
	}
}

func run(addr, dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("创建数据目录: %w", err)
	}
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	defer database.Close()

	st := &store{db: database}
	authSvc, err := newAuth(st, dataDir)
	if err != nil {
		return fmt.Errorf("初始化鉴权: %w", err)
	}
	panel := newPanelHub(authSvc)
	agents := newAgentHub(st, panel)
	// 面板建连回执需要在线快照；bindSnapshot 必须先于首个 WS 连接
	panel.bindSnapshot(agents.onlineMap)
	apiSvc := newAPI(st, authSvc, agents)

	root := http.NewServeMux()
	// healthz 供外部探活（compose/监控）使用，无鉴权、需在 SPA fallback 之前注册
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		jsonWrite(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	root.Handle("/api/", apiSvc.routes())
	root.HandleFunc("/ws/agent", agents.HandleAgentWS)
	root.HandleFunc("/ws/panel", panel.HandlePanelWS)
	root.Handle("/", spaHandler(webui.DistFS))
	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequest(root),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[hub] TarShip v%s 启动完成，监听 %s（数据目录 %s）", version.Version, addr, dataDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[hub] HTTP 服务异常退出: %v", err)
		}
	}()

	// 优雅关闭：等待连接排空，Agent 重连由其指数退避兜底
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("[hub] 收到退出信号，正在关闭…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// spaHandler 服务内嵌面板静态文件；未命中的路径回退 index.html，交由前端路由接管。
func spaHandler(dist fs.FS) http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		log.Fatalf("[hub] 内嵌面板资源缺失: %v", err)
	}
	fileServer := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			// SPA 前端路由：刷新 /servers 等深链时必须回退到入口页
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

// logRequest 打印一行访问日志，便于裸机部署排查（容器下走 docker logs）。
func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		// 静态资源与心跳类请求量小，M0 全量记录，M1 视规模再降噪
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
