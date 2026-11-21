# 更新记录

## v0.2.0 · M0 合并重构（2026-11-21）

后端内核与安全设计以社区实现（版本 B）为底，与原实现（版本 A）的交付能力合并。

**后端（版本 B 内核）**

- 分层重构：`internal/config`（参数 > 环境变量 > 默认值）、`internal/proto`（协议独立包）、`internal/store`（SQL 文件式迁移 + 单连接串行化避免锁冲突）
- 并发正确性：全部测试在 `go test -race` 下通过（修复了 `agentConn` 字段跨 goroutine 读写与面板连接并发写两个数据竞争）
- Agent 重连：稳定连接 1 分钟后退避复位（修复退避只增不减导致最终固定 60 秒），带抖动；被同 token 新连接顶替时明确退出而非无限互踩
- 协议：新增 `protocol_version` 与 `welcome` 应答，不兼容时明确拒绝；消息大小上限 64KB
- 安全：首次启动一次性 setup token（日志发布，防公网抢注）；登录限速（IP 10 分钟 5 次失败锁定）；时序探测防护；`SameSite=Strict` + HTTPS 下 `Secure`；CSRF Origin 校验；安全响应头；Agent 默认拒绝明文 `ws://`（`--allow-insecure-ws` 仅调试）
- Agent token 支持环境变量 `TARSHIP_TOKEN` 注入，不再出现在进程列表
- 密码校验上限修正为 72 字节（bcrypt 实际上限）
- 测试从 10 个增至 30+，含端到端流程

**自版本 A 并入**

- 发版与镜像流水线：`release.yml`（tag → 双架构二进制 + 校验和）、`docker.yml`（latest=edge 策略的镜像发布）、`ci.yml`（含 `-race`）
- 仪表盘「版本落后」统计：语义化版本比较（Agent < Hub 才计入，开发版不参与），面板统计卡与升级提示
- 安装命令：服务端下载 + `/tmp` 原子替换（避免覆盖运行中二进制的 ETXTBSY）+ token 环境文件注入 + `restart`（升级重装兼容），支持 amd64/arm64 切换
- `TARSHIP_JWT_SECRET` 环境变量覆盖（至少 32 字节）
- scratch + CA 证书镜像（19MB），融合 HEALTHCHECK 与 nonroot 数据卷
- `web/dist/.gitkeep`：全新 clone 后无需构建前端即可通过 `go build`/`go vet`

**兼容性说明**

- v0.1.x 数据库可直接升级：管理员账号、服务器与 token 全部保留
- 服务器标签自动兼容旧逗号分隔格式
- 升级到 nonroot 运行需一次性调整数据卷属主（见 README 升级说明）
- JWT 密钥来源变更（文件 → 数据库），升级后需重新登录一次
- v0.1.x Agent 与 v0.2.0 Hub 协议不兼容，Hub 升级后 Agent 需重新执行安装命令

## v0.1.4 · 安装命令覆盖修复（2026-11-19）

- 安装命令改为先下载 /tmp 再 mv 原子替换：直接 `-o` 覆盖正在运行的 Agent 二进制会触发 ETXTBSY 写入保护

## v0.1.3 · 安装命令升级兼容（2026-11-19）

- 安装命令结尾改为 daemon-reload + enable + restart：`enable --now` 对已运行服务不会重启

## v0.1.2 · 版本比较修正（2026-11-19）

- 仪表盘「版本落后」改用语义化版本比较，消除 dev 版误报
- 镜像标签策略：`latest` 指向最新稳定 Release，main 分支构建打 `edge`

## v0.1.1 · 弹窗修复与 CI（2026-11-19）

- 修正 Tailwind content 扫描路径（HeroUI v2.8 子包嵌套 + `.mjs`），修复弹窗渲染在视口外不可见
- 新增 GitHub Actions：CI 质量门禁、Docker 自动构建、发版流水线

## v0.1.0 · M0 骨架（2026-11-18）

- 单二进制双角色（`hub`/`agent`）、Agent 注册/心跳/在线判定、JWT 登录、React 面板骨架与路由、实时在线状态
- 纯 Go SQLite（modernc.org/sqlite），多阶段 Docker 构建
