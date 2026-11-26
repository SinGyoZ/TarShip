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

### 当前版本（M1）已交付

| 能力 | 说明 |
| --- | --- |
| 面板认证 | 首启一次性 setup token 初始化、bcrypt 密码哈希、JWT HttpOnly Cookie（24h）、登录限速 |
| 服务器管理 | 添加（生成 token + 一键安装命令）、删除、token 重置 |
| 实时状态 | WebSocket 事件驱动，上/下线秒级反映到面板 |
| 存储器 | local 目录与 S3 兼容端点（MinIO/R2/B2），连通性测试（S3 由 Hub 验证，local 由 Agent 实测可写） |
| 备份任务 | 多目录、doublestar 排除规则（支持 `**`）、绑定服务器与存储器 |
| 手动备份 | Agent 端 tar 流式打包 → zstd 压缩 → **直传存储器**（S3 流式 multipart，数据不落 Agent 本地） |
| 完整性校验 | 产物 sha256 全程流式计算，与面板登记值可比对 |
| 实时进度 | 运行状态经 WebSocket 推送，任务页/仪表盘实时可见 |
| 备份库 | 产物登记（对象键、大小、sha256）与完整运行历史，产物为标准 `.tar.zst` 可手工恢复 |
| 健壮性 | Agent 断线/Hub 重启时未完成运行自动标记失败，同任务并发防护 |

### 路线图

| 里程碑 | 交付内容 | 状态 |
| --- | --- |
| M0 骨架 | 双角色入口、注册/心跳/在线判定、JWT 登录、面板骨架 | ✅ |
| M1 备份闭环（MVP） | 存储管理（S3 + 本地）、任务 CRUD、手动备份、tar+zstd 流式直传、实时进度、sha256 校验 | ✅ 当前版本 |
| M2 调度自动化 | cron 调度、错过补跑、失败重试、webhook 通知、保留策略与回收站 | 🔜 |
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

1. 面板 →「服务器」→「添加服务器」，填写名称
2. 创建后弹窗展示 **Agent Token（仅此一次）** 与安装命令，支持按目标机器架构切换（`amd64`/`arm64`），点击复制
3. 在目标服务器上执行——**整块复制、一次粘贴、回车完成**。命令是自包含的一条复合命令（`&&` 串联），内部完成：下载二进制 → sha256 校验 → 原子替换 → 写入 600 权限环境文件 → systemd 注册并启动：

```bash
curl -fsSL https://github.com/SinGyoZ/TarShip/releases/latest/download/tarship-linux-amd64 -o /tmp/tarship.new \
  && curl -fsSL https://github.com/SinGyoZ/TarShip/releases/latest/download/checksums.txt -o /tmp/tarship.sha256 \
  && grep "tarship-linux-amd64" /tmp/tarship.sha256 | awk '{print $1"  tarship.new"}' | (cd /tmp && sha256sum -c -) \
  && sudo chmod +x /tmp/tarship.new \
  && sudo mv /tmp/tarship.new /usr/local/bin/tarship \
  && sudo install -m 600 /dev/null /etc/tarship-agent.env \
  && printf '%s\n' 'TARSHIP_SERVER=wss://hub.example.com:8080' 'TARSHIP_TOKEN=<TOKEN>' | sudo tee /etc/tarship-agent.env >/dev/null \
  && printf '%s\n' '[Unit]' 'Description=TarShip Agent' 'After=network-online.target' 'Wants=network-online.target' '' '[Service]' 'EnvironmentFile=/etc/tarship-agent.env' 'ExecStart=/usr/local/bin/tarship agent' 'Restart=always' 'RestartSec=5' '' '[Install]' 'WantedBy=multi-user.target' | sudo tee /etc/systemd/system/tarship-agent.service >/dev/null \
  && sudo systemctl daemon-reload \
  && sudo systemctl enable tarship-agent >/dev/null \
  && sudo systemctl restart tarship-agent \
  && echo '安装完成'
```

卸载（同样整块复制执行；**不影响已产生的备份数据**）：

```bash
sudo systemctl disable --now tarship-agent 2>/dev/null; \
  sudo rm -f /etc/systemd/system/tarship-agent.service /etc/tarship-agent.env /usr/local/bin/tarship \
  && sudo systemctl daemon-reload \
  && echo '已卸载'
```

> 说明：安装与升级重装是同一条命令（覆盖运行中的二进制走 mv 原子替换）；token 会进入 shell history，属一次性安装场景可接受，泄露可随时在面板重置。明文 `ws://` 默认拒绝，面板走 http 时生成的命令已附带调试开关。

4. 回到面板，几秒内该服务器亮起「在线」状态

### 3. 验证

面板「仪表盘」显示在线统计；「服务器」页状态实时刷新——杀掉 Agent 进程，页面几秒内自动变离线；重新拉起，自动恢复在线。

## 使用教程

### 登录与初始化

- **首次启动**：Hub 在日志里发布一次性 setup token（防公网抢注——只有能看到服务器日志的人才能创建管理员）。打开面板把它连同样用户名、密码填入初始化页；用户名 3-32 位字母数字及 `_ . -`，密码 8-72 字节。
- **会话**：JWT 存于 HttpOnly Cookie（`SameSite=Strict`，HTTPS 下自动加 `Secure`），24 小时有效。
- **登录限速**：同一 IP 10 分钟内 5 次失败后锁定，成功登录后清零。

### 服务器管理

| 操作 | 说明 |
| --- | --- |
| 添加服务器 | 生成一次性 Token 与安装命令；Token 明文只在创建/重置时展示一次，数据库只存 SHA-256 |
| 在线状态 | Agent 心跳 30 秒；Hub 90 秒无消息判离线。状态经 WebSocket 推送到面板，无需刷新 |
| 重置 Token | 旧 Agent 立即失联（用于疑似泄露），用新 Token 重新部署即可 |
| 删除服务器 | 立即断开其 Agent 并删除注册记录，不可撤销 |

### Agent 版本提示

服务器列表展示各 Agent 版本号，仪表盘统计「版本落后」数量（语义化比较，Agent 低于 Hub 才计入；开发版不参与）。Agent 不做自动升级，在服务器页重置 token 后重新执行安装命令即可。

### 心跳与重连行为

- Agent 心跳 30 秒，超过 90 秒无消息判定离线；断线后指数退避重连（1s 起，封顶 60s，带抖动），**稳定连接 1 分钟后退避自动复位**。
- 被同 token 的新连接顶替时旧 Agent 明确退出（不做无限互踩）；Hub 重启后全部 Agent 自动回归在线，服务器注册记录持久保存于 SQLite。

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

Agent 直接读取宿主机目录，裸机部署最简单。安装命令可直接在面板 token 弹窗中复制（支持 `amd64`/`arm64` 切换）。

`agent` 子命令参数与环境变量（优先级：命令行 > 环境变量）：

| 参数 / 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `--server` / `TARSHIP_SERVER` | （必填） | Hub 地址，必须以 `wss://` 开头 |
| `--token` / `TARSHIP_TOKEN` | （必填） | 面板生成的 Agent Token；推荐用环境变量，避免出现在进程列表 |
| `--allow-insecure-ws` / `TARSHIP_ALLOW_INSECURE_WS` | `false` | 允许明文 `ws://` 连接，仅限本地调试（token 会明文传输） |

Agent 心跳固定 30 秒，主机名取操作系统值，不再提供覆盖参数。

### Agent · Docker 方式（备选）

```yaml
services:
  tarship-agent:
    image: singyoku/tarship:latest
    command: agent
    environment:
      TARSHIP_SERVER: wss://hub.example.com:8080
      TARSHIP_TOKEN: <TOKEN>
    volumes:
      - /var/www:/var/www:ro          # 被备份的目录：容器内路径与任务 paths 保持一致
      # 使用 local 存储器时，必须把存储目录也挂进来（路径与存储器 root_dir 一致），
      # 否则产物会写进容器内部，容器删除即丢失：
      # - /srv/tarship-backups:/srv/tarship-backups
    restart: unless-stopped
```

> [!WARNING]
> **local 存储器 + 容器化 Agent 的关键约定**：`root_dir` 是 Agent 视角路径。容器化 Agent 必须把宿主机存储目录以同一路径挂载进容器，否则备份会"成功"写入容器可写层，容器删除即丢失（推荐容器场景直接用 S3 存储器规避）。

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
| `TARSHIP_JWT_SECRET` | 可选，面板 JWT 签名密钥（至少 32 字节）；未设置时首次启动自动生成并存入数据库 |
| `TARSHIP_ADDR` / `TARSHIP_DATA` | hub 的监听地址 / 数据目录，与 `--addr` / `--data` 参数等价 |
| `TARSHIP_TRUST_PROXY` | Hub 在可信反代之后时设为 `true`，登录限速按真实客户端 IP 计算 |
| `TARSHIP_SERVER` / `TARSHIP_TOKEN` / `TARSHIP_ALLOW_INSECURE_WS` | Agent 连接配置，见上方参数表 |

> [!NOTE]
> Agent Token 经 `Authorization` 请求头传输，不落在 URL 与常规访问日志；传输层安全由 `wss://` 保证。Hub 内置 `healthcheck` 子命令供容器健康检查使用。

## 本地构建与开发

依赖：Go 1.27+、Node.js 22+（含 npm；Vite 8 对 Node 有版本下限要求）。

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
镜像由 CI 自动构建：`latest` = 最新稳定 Release，`edge` = main 开发版。升级：`docker compose pull && docker compose up -d`；确认容器内版本可执行 `docker compose exec tarship /usr/local/bin/tarship version`（应显示版本号，如 `v0.2.1`）。仪表盘的「版本落后」按语义化版本比较，Agent 版本低于 Hub 才计入；开发版（dev）不参与比较。

**Q：Token 忘了保存怎么办？**
无法找回明文（库里只存 SHA-256）。在服务器列表点「重置 Token」生成新的，旧 Agent 自动失联，用新 Token 重新部署。

**Q：Agent 一直离线？**
1. 确认 `--server` 地址从 Agent 所在机器可达（`curl -I http://hub:8080/healthz`）；
2. 确认 Token 没被重置过；
3. 查看 Agent 日志：`journalctl -u tarship-agent -f`（systemd）或 `docker logs -f <容器>`。

**Q：Hub 启动报「数据目录不可写」？**
容器默认以 UID 65532 运行，挂载目录（尤其宿主机 bind mount 的目录）属主是 root 时无法写入。任选其一修复：
```bash
chown -R 65532:65532 <数据目录>      # 方式一：调整属主（推荐）
# 方式二：compose 里删除 user: "65532:65532" 行，改回 root 运行
```
命名卷用户可执行：`docker run --rm -v <项目名>_tarship-hub-data:/data alpine chown -R 65532:65532 /data`

**Q：为什么镜像是 scratch、没有 shell？**
为了极致体积（约 18MB）与最小攻击面。排障用 `docker logs`；需要进入文件系统可用 `docker export`。Hub 对外提供 `/healthz` 探活端点（无需登录）。

**Q：数据目录怎么备份？**
`/data`（或 `--data` 指定目录）里只有一个关键文件 `tarship.db`（管理员账号、服务器、JWT 密钥全在库里）。SQLite 已启用 WAL，建议先停容器再复制，或使用 `sqlite3 .backup`；热拷贝时请连同 `tarship.db-wal`、`tarship.db-shm` 一起复制。

**Q：能换端口吗？**
可以：Hub 用 `--addr :9090`（compose 里改 ports 映射），Agent 连接地址相应变更。

## 更新记录

完整历史见 [CHANGELOG.md](./CHANGELOG.md)。

### v0.3.2 · 一键命令改为面板自包含生成（2026-11-26）

- 回退 install-agent.sh 脚本方案：安装命令由面板自包含生成（`&&` 串联的一条复合命令），整块复制、一次粘贴、回车完成；不再依赖外部脚本资产
- 修正「登录与初始化」旧文案（与 setup token 机制对齐）

### v0.3.1 · 安装体验与权限指引（2026-11-26）

- Agent 安装命令与卸载命令同展示于 token 弹窗（不影响已产生的备份数据）
- Hub 启动时探测数据目录可写性，权限错误直接输出三种修复指引，不再吐 `unable to open database file (14)` 裸错误码

### v0.3.0 · M1 备份闭环（2026-11-21）

- 存储器管理（local + S3 兼容，连通性测试，secret 不回显）
- 备份任务（多目录、`**` 排除规则）与手动备份闭环：tar 流式打包 → zstd → **Agent 直传存储器**（S3 流式 multipart），sha256 全程校验
- 实时进度与运行徽章；备份库（产物登记 + 运行历史）
- Agent 断线 / Hub 重启时未完成运行自动清场；同任务并发防护

### v0.2.1 · 文档与细节修正（2026-11-21）

- README：容器内二进制路径、备份 FAQ（JWT 密钥已入库）、构建依赖版本修正
- 安装命令：下载后用 Release 校验和验证 sha256；`mv` 步骤补 `sudo`
- release.yml：Node 统一为 22，校验和去路径前缀
- 入口内嵌 `time/tzdata`（scratch 无时区数据，为 M2 cron 铺垫）

### v0.2.0 · M0 合并重构（2026-11-21）

**后端内核重构（并发正确性与安全）**

- 全部测试 `go test -race` 通过，含端到端流程（30+ 测试）
- 首次启动一次性 setup token 初始化管理员（防公网抢注）、登录限速、时序探测防护
- Agent 默认拒绝明文 `ws://`；token 支持环境变量注入，不出现在进程列表
- 协议新增 `protocol_version`；重连退避稳定后自动复位；被顶替时明确退出

**自 v0.1.x 并入与保留**

- 发版与镜像流水线（tag → 双架构二进制 + 校验和 + latest/edge 镜像）
- 仪表盘「版本落后」语义化统计、安装命令（下载原子替换 + 环境文件 + 架构切换）
- `TARSHIP_JWT_SECRET` 环境变量、scratch + CA 证书镜像（19MB，含 HEALTHCHECK 与 nonroot）

**升级注意**

- 管理员账号、服务器与 token 全部保留，数据库直接升级
- Hub 改为 nonroot 运行：从 v0.1.x 升级需先把数据卷属主改为 65532（一次性）：

```bash
docker compose down
docker run --rm -v tarship_tarship-hub-data:/data alpine chown -R 65532:65532 /data
docker compose pull && docker compose up -d
```

- JWT 密钥来源变更，升级后需重新登录一次
- v0.1.x Agent 与 v0.2.0 Hub 协议不兼容：Hub 升级后请对每台服务器重新执行面板生成的安装命令

### v0.1.4 及更早

见 [CHANGELOG.md](./CHANGELOG.md)。

---

## License

[MIT](./LICENSE)
