# 🚢 TarShip

**把指定服务器的指定目录打包成标准 `.tar.zst`，运到指定存储器；全程由网页面板控制。**

TarShip 是一艘运输 tar 包的船：Agent 在源服务器上打包、压缩、直传存储器，Hub 只负责调度与元数据，备份数据**从不流经控制面**。产物是标准 `.tar.zst` 归档，脱离 TarShip 也能手工恢复，永不锁死。

> [!IMPORTANT]
> **当前版本为 M0（骨架版）**：已交付面板登录、服务器接入与实时在线状态。
> 备份核心能力（存储器、任务、备份与恢复）将在 M1 起逐里程碑交付，见下方 [更新记录与路线图](#更新记录)。

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=white)
![Docker](https://img.shields.io/badge/Docker%20镜像-约18MB-2496ED?logo=docker&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-纯Go实现(无CGO)-044A64?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## 功能特点

- **控制面与数据面分离** —— Hub 只管调度和元数据，跑在 1C1G 的小机器上也扛得住几百 GB 的备份；数据由 Agent 直传存储器。
- **出站长连接，零入站端口** —— Agent 主动连出 Hub（WebSocket），源服务器无需对公网开放任何新端口。
- **单二进制，双角色** —— `tarship hub` 与 `tarship agent` 是同一个程序，共享一套打包/存储/协议代码；Docker 镜像 `singyoku/tarship` 同一镜像靠 command 区分。
- **网页面板内嵌于二进制** —— React SPA 经 `go:embed` 打进 Hub，单文件部署，无需额外前端资产目录。
- **实时在线状态** —— Agent 心跳 30 秒，超 90 秒无消息自动判离线；面板经 WebSocket 秒级刷新，无需手动刷新页面。
- **纯 Go SQLite** —— `modernc.org/sqlite` 无 CGO 依赖，静态编译进 scratch 镜像，最终镜像约 18MB。
- **凭据最小化** —— Agent token 明文仅展示一次，库中只存 SHA-256；泄露可随时重置令旧 Agent 立即失联。
- **断线自愈** —— Agent 掉线按指数退避自动重连，Hub 重启后全部 Agent 自动回归在线。

### 当前版本（M0）已交付

| 能力 | 说明 |
| --- | --- |
| 面板认证 | 首启初始化向导、bcrypt 密码哈希、JWT HttpOnly Cookie（24h） |
| 服务器管理 | 添加（生成 token + 一键安装命令）、删除、token 重置 |
| 实时状态 | WebSocket 事件驱动，上/下线秒级反映到面板 |
| 仪表盘 | 服务器总数、在线/离线、Agent 版本落后统计 |
| Agent | 长连接、hello/heartbeat、指数退避重连、hostname 上报 |

### 路线图

| 里程碑 | 交付内容 | 状态 |
| --- | --- | --- |
| M0 骨架 | 双角色入口、注册/心跳/在线判定、JWT 登录、面板骨架 | ✅ 当前版本 |
| M1 备份闭环（MVP） | 存储管理（S3 + 本地）、任务 CRUD、手动备份、tar+zstd 流式直传、实时进度、sha256 校验 | 🔜 |
| M2 调度自动化 | cron 调度、错过补跑、失败重试、webhook 通知、保留策略与回收站 | ⏳ |
| M3 恢复闭环 | 一键恢复（原位/另存）、恢复前快照、一键回滚、解包安全规则 | ⏳ |
| M4 加密与发版 | 任务级 age 加密、私钥导出、reindex 灾难恢复、CI 全流程、v1.0.0 | ⏳ |

## 架构

```
┌─────────────────┐
│   浏览器面板     │  React SPA（go:embed 内嵌于 Hub 二进制）
└────────┬────────┘
         │ REST API + /ws/panel 实时事件（JWT HttpOnly Cookie）
┌────────▼─────────────────────────────┐
│              Hub（控制面）              │  tarship hub
│  调度 · REST/WS API · SQLite · 面板   │  --addr :8080 --data /data
└────────▲─────────────────────────────┘
         │ 出站 WebSocket 长连接（wss://，Agent 主动连出）
┌────────┴───────┬────────────────┬──────────────┐
│ Agent · VPS-01 │ Agent · VPS-02 │ Agent · VPS-N │  tarship agent
└────────────────┴───────┬────────┴──────────────┘
                         │ tar+zstd 流式直传（M1 起接入）
                 ┌───────▼────────┐
                 │   存储器        │  S3 兼容（R2/B2/MinIO/OSS）· 本地磁盘
                 └────────────────┘
```

一次典型备份的数据路径（M1 交付后）：**Agent 收到指令 → 本地 tar 流式打包 → 直接写入 S3 → 向 Hub 上报元数据**。数据不流经 Hub。

## 快速开始

### 1. 启动 Hub（Docker，推荐）

```bash
git clone https://github.com/SinGyoZ/TarShip.git
cd TarShip

# 直接拉取 Docker Hub 镜像启动（CI 自动构建，随时最新）
docker compose up -d

# 或从源码构建（修改过代码 / 需要本地定制时）
docker compose up -d --build
```

启动后浏览器访问 `http://<服务器IP>:8080`，首次进入会引导你创建管理员账号。

### 2. 接入第一台服务器

1. 面板 →「服务器」→「添加服务器」，填写名称、标签与 Hub 地址
2. 创建后弹窗展示 **Agent Token（仅此一次）** 与一键安装命令，点击复制
3. 在目标服务器上以 root 执行安装命令：

```bash
curl -fsSL https://github.com/SinGyoZ/TarShip/releases/latest/download/tarship-linux-amd64 -o /usr/local/bin/tarship
chmod +x /usr/local/bin/tarship
cat > /etc/systemd/system/tarship-agent.service <<'EOF'
[Unit]
Description=TarShip Agent
After=network-online.target
[Service]
ExecStart=/usr/local/bin/tarship agent --server wss://hub.example.com:8080 --token <TOKEN>
Restart=always
[Install]
WantedBy=multi-user.target
EOF
systemctl enable --now tarship-agent
```

4. 回到面板，几秒内该服务器亮起「在线」状态

### 3. 验证

面板「仪表盘」显示在线统计；「服务器」页状态实时刷新——杀掉 Agent 进程，页面几秒内自动变离线；重新拉起，自动恢复在线。

## 使用教程

### 登录与初始化

- **首次启动**：面板检测到未初始化时自动进入向导，设置用户名（3-32 字符）与密码（至少 8 位），完成后自动登录。
- **会话**：JWT 存于 HttpOnly Cookie，24 小时有效；退出登录即失效。

### 服务器管理

| 操作 | 说明 |
| --- | --- |
| 添加服务器 | 生成一次性 Token 与安装命令；Token 明文只在创建/重置时展示一次，数据库只存 SHA-256 |
| 在线状态 | Agent 心跳 30 秒；Hub 90 秒无消息判离线。状态经 WebSocket 推送到面板，无需刷新 |
| 重置 Token | 旧 Agent 立即失联（用于疑似泄露），用新 Token 重新部署即可 |
| 删除服务器 | 立即断开其 Agent 并删除注册记录，不可撤销 |

### Agent 版本提示

服务器列表展示各 Agent 版本号，仪表盘统计「版本落后」数量（Agent 版本 ≠ Hub 版本时计入）。Agent 不做自动升级，按需重新执行安装命令即可。

### 心跳与重连行为

- Agent 断线后按指数退避重连（1s → 2s → 4s … 封顶 60s），重连成功自动重新上线。
- Hub 重启后全部 Agent 会自动回归在线，服务器注册记录与统计持久保存于 SQLite。

## 部署教程

### Hub · Docker Compose（推荐）

```yaml
services:
  tarship:
    image: singyoku/tarship:latest
    build: .
    command: hub
    ports:
      - "8080:8080"
    volumes:
      - tarship-hub-data:/data
    restart: unless-stopped
volumes:
  tarship-hub-data:
```

- 数据目录 `/data`（SQLite + JWT 密钥）全部在卷内，**请务必对它做备份**（例如用 TarShip 自身把宿主机的卷目录纳入某台机器的备份任务）。
- 镜像由 CI 自动构建推送 Docker Hub：`latest` 始终指向最新稳定 Release（tag 构建），`edge` 跟随 main 开发版；升级执行 `docker compose pull && docker compose up -d`。
- 本地源码构建部署：`docker compose up -d --build`。

### Hub · 裸机运行

```bash
# 工作目录：TarShip 项目根
make            # 先构建前端再编译二进制（产物 bin/tarship）
./bin/tarship hub --addr :8080 --data /var/lib/tarship
```

`hub` 子命令参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--addr` | `:8080` | HTTP 监听地址 |
| `--data` | `/data` | 数据目录（SQLite、JWT 密钥），裸机部署请指定可写目录 |

### Agent · 裸机 systemd（推荐）

Agent 直接读取宿主机目录，裸机部署最简单。安装命令可直接在面板添加服务器向导中复制（`amd64`/`arm64` 可选）。

`agent` 子命令参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--server` | （必填） | Hub 地址，支持 `ws:// wss:// http:// https://` 写法 |
| `--token` | （必填） | 面板生成的 Agent Token |
| `--hostname` | 操作系统主机名 | 上报到面板的显示名 |
| `--heartbeat` | `30s` | 心跳间隔（最小 5s） |

### Agent · Docker 方式（备选）

```yaml
services:
  tarship-agent:
    image: singyoku/tarship:latest
    command: agent --server wss://hub.example.com:8080 --token <TOKEN>
    volumes:
      - /var/www:/var/www:ro   # 按需挂载要备份的目录（M1 起生效）
    restart: unless-stopped
```

### TLS 与反向代理

生产环境建议前置 Caddy/Nginx 终结 TLS，Agent 与浏览器均走 `wss://` / `https://`：

```nginx
server {
    listen 443 ssl;
    server_name hub.example.com;
    # ssl_certificate / ssl_certificate_key …
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;      # WebSocket 必需
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 300s;                      # 长连接不被掐断
    }
}
```

### 环境变量

| 变量 | 说明 |
| --- | --- |
| `TARSHIP_JWT_SECRET` | 可选，面板 JWT 签名密钥（至少 32 字节）；未设置时首次启动自动生成并保存到 `<data>/jwt.secret`（0600） |

> [!NOTE]
> Agent Token 经 `Authorization` 请求头传输，不会出现在 URL 与常规访问日志中；传输层安全由 `wss://` 保证。

## 本地构建与开发

依赖：Go 1.22+、Node.js 20+（含 npm）。

```bash
# 工作目录：TarShip 项目根
make web      # 构建前端（web/dist，go:embed 依赖）
make build    # 编译 bin/tarship
make test     # go vet + 单元测试
make run      # 本地起一个开发用 Hub（数据目录 .data，端口 8080）
```

前端开发（热更新，`web/` 目录）：

```bash
npm run dev          # Vite 开发服务器 :5173，/api 与 /ws 代理到 :8080
```

目录结构：

```
TarShip/
├── cmd/tarship/          # 入口：hub / agent 子命令分发
├── internal/
│   ├── hub/              # 控制面：API、WS、SQLite、JWT
│   ├── agent/            # 代理端：长连接、心跳、重连
│   ├── db/               # SQLite（modernc.org/sqlite）+ 版本化迁移
│   └── version/          # 版本号（-ldflags 注入）
├── web/                  # React 面板源码 + go:embed
│   ├── dist/             # 前端构建产物（make web 生成）
│   └── src/              # 页面与组件
├── Dockerfile            # 多阶段构建，最终 scratch 层约 18MB
└── docker-compose.yml
```

## 常见问题

**Q：怀疑面板跑的不是最新版？**
镜像由 CI 自动构建：`latest` = 最新稳定 Release，`edge` = main 开发版。升级：`docker compose pull && docker compose up -d`；确认容器内版本可执行 `docker compose exec tarship /tarship version`（应显示版本号，如 `v0.1.2`）。仪表盘的「版本落后」按语义化版本比较，Agent 版本低于 Hub 才计入；开发版（dev）不参与比较。

**Q：Token 忘了保存怎么办？**
无法找回明文（库里只存 SHA-256）。在服务器列表点「重置 Token」生成新的，旧 Agent 自动失联，用新 Token 重新部署。

**Q：Agent 一直离线？**
1. 确认 `--server` 地址从 Agent 所在机器可达（`curl -I http://hub:8080/healthz`）；
2. 确认 Token 没被重置过；
3. 查看 Agent 日志：`journalctl -u tarship-agent -f`（systemd）或 `docker logs -f <容器>`。

**Q：为什么镜像是 scratch、没有 shell？**
为了极致体积（约 18MB）与最小攻击面。排障用 `docker logs`；需要进入文件系统可用 `docker export`。Hub 对外提供 `/healthz` 探活端点（无需登录）。

**Q：数据目录怎么备份？**
`/data`（或 `--data` 指定目录）里只有 `tarship.db` 与 `jwt.secret` 两个关键文件。直接复制这两个文件即可（SQLite 已启用 WAL，建议先停容器再复制，或使用 `sqlite3 .backup`）。

**Q：能换端口吗？**
可以：Hub 用 `--addr :9090`（compose 里改 ports 映射），Agent 连接地址相应变更。

## 更新记录

### v0.1.2 · 版本比较修正（2026-11-19）

**修复**

- 仪表盘「版本落后」改用语义化版本比较：Agent 版本低于 Hub 才计入；任一方为非 semver（如 dev 开发版）不计入，消除「装了最新版却提示落后」的误报

**变更**

- 镜像标签策略：`latest` 始终指向最新稳定 Release（tag 构建），main 分支构建改打 `edge` 标签

### v0.1.1 · 弹窗修复与 CI（2026-11-19）

**修复**

- 修正 Tailwind content 扫描路径：HeroUI v2.8 的组件类名分散在嵌套子包的 `.mjs` chunk 中，原配置扫描的 `@heroui/theme` 路径已不存在，导致 Modal 遮罩层 `fixed/inset-0/z-50` 等 utility 类未生成 CSS、弹窗被排到视口外不可见（表现为点击「添加服务器」无反应）

**新增**

- GitHub Actions CI 质量门禁：前端构建 + go vet + 单测
- GitHub Actions Docker 自动构建：push main 产出 `latest` + `sha-*` 标签，v* tag 追加版本标签；`docker compose up -d` 可直接拉取最新镜像
- 发版流水线：tag v* 自动产出 linux amd64/arm64 静态二进制与 sha256 校验和并发布 GitHub Release，面板「添加服务器」生成的安装命令自此可直接下载

### v0.1.0 · M0 骨架（2026-11-18）

**新增**

- 单二进制双角色：`tarship hub` / `tarship agent` 子命令，参数齐全
- 面板认证：首启初始化向导、bcrypt、JWT HttpOnly Cookie（24h）
- 服务器管理：添加（一次性 Token + 一键安装命令）、删除、Token 重置
- Agent 长连接：Bearer Token 认证、hello/heartbeat、30s 心跳、90s 离线判定、指数退避重连（封顶 60s）
- 面板实时状态：`/ws/panel` 事件广播 + 建连即回执在线快照（panel_hello）
- React 面板骨架：登录、仪表盘、服务器页、任务/备份库/存储器/设置占位路由
- 基础设施：纯 Go SQLite（modernc.org/sqlite）+ 版本化迁移、多阶段 Docker 构建（scratch 最终层约 18MB）、docker compose 模板、Makefile、单元测试

---

## License

[MIT](./LICENSE)
