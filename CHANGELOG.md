# 更新记录

## v0.3.2 · 一键命令改为面板自包含生成（2026-11-26）

**变更**

- 回退 v0.3.1 的 install-agent.sh 托管脚本方案：用户反馈安装入口应是“一条命令完成安装”，不需要额外的脚本资产与下载环节
- 面板生成的安装命令改为自包含复合命令（`&&` 串联）：整块复制、一次粘贴、回车完成下载→sha256 校验→原子替换→600 权限环境文件→服务注册与重启；任一步失败立即中止；卸载命令同样自包含，不影响已产生的备份数据
- 修正 README「登录与初始化」旧文案（与 v0.2.0 起 setup token 机制对齐）

## v0.3.1 · 安装体验与权限指引（2026-11-26）

**新增**

- `scripts/install-agent.sh`：Agent 一键安装/卸载脚本，随 Release 发布为固定资产（纳入 checksums）；面板生成的安装命令收敛为单行 `curl | sh`，内部完成下载 → sha256 校验 → 原子替换 → 600 权限环境文件 → systemd 注册与重启；`--uninstall` 停止服务并删除二进制与配置，不触碰已产生的备份数据；明文 `ws://` 默认拒绝，需显式 `--allow-insecure-ws`
- 面板 token 弹窗同步展示一键安装命令与卸载命令

**修复**

- Hub 启动探测数据目录可写性：权限不足时输出人话修复指引（chown 65532 / 去掉 nonroot / 换可写目录），替代裸 SQLite 错误码（用户实测踩坑 `unable to open database file (14)`）
- README FAQ 补充权限问题条目与修复命令

## v0.3.0 · M1 备份闭环（2026-11-21）

**新增**

- 存储器管理：local 目录与 S3 兼容端点（MinIO/R2/B2，minio-go 驱动）；连通性测试——S3 由 Hub 直接 HeadBucket，local 由 Agent 实测目录可写；secret 不回显，更新留空沿用旧值；被任务引用的存储器不可删除
- 备份任务：多目录、doublestar 排除规则（支持 `**` 递归 glob）、绑定目标服务器与存储器；Agent 离线时禁止触发
- 手动备份闭环：Agent 端 tar 流式打包（排除剪枝、符号链接保留、不可读条目跳过）→ zstd 压缩 → 直传存储器（S3 流式 multipart，数据不落 Agent 本地；local 临时文件原子替换）；产物对象键 `backups/{server_id}/{task_id}/{时间戳}.tar.zst`
- 完整性：sha256 全程流式计算，与面板登记值可比对
- 实时进度：运行状态经 WebSocket 推送（进度节流上报），任务页运行徽章、仪表盘最近备份实时更新
- 备份库：产物登记（对象键/大小/sha256）与完整运行历史
- 健壮性：同任务并发防护；Agent 断线时未完成运行自动终止；Hub 重启时遗留 running 清场
- 面板新页面：存储器（列表/创建/编辑/测试）、任务（列表/创建/编辑/触发）、备份库（运行历史 + 产物表）

**已知语义**

- local 存储器路径为 Agent 视角：容器化 Agent 必须把存储目录同路径挂载进容器，否则产物写入容器可写层（文档与 UI 提示已标注）

**依赖**

- 新增 minio-go/v7（S3）、klauspost/compress（zstd）、doublestar/v4（排除规则）

## v0.2.1 · 文档与细节修正（2026-11-21）

- README：容器内二进制路径修正为 `/usr/local/bin/tarship`（原 `/tarship` 为 v0.1.x 路径）；备份 FAQ 修正——v0.2.0 起 JWT 密钥存于数据库，数据目录仅需备份 `tarship.db`；构建依赖更正为 Go 1.27+ / Node 22+
- 安装命令：下载后用 Release 的 `checksums.txt` 做 sha256 校验再落地，防传输损坏与劫持；`mv` 步骤补上 `sudo` 与其余步骤保持一致
- release.yml：Node 版本统一为 22；校验和改为不带路径前缀，便于直接 `sha256sum -c`
- 入口内嵌 `time/tzdata`：scratch 镜像无系统时区数据，为 M2 的 cron 时区调度铺垫

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
