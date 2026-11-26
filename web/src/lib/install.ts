/**
 * Agent 一键安装/卸载命令生成。
 *
 * 安装收敛为托管在 Release 的脚本（install-agent.sh），面板只生成单行命令；
 * 脚本内部完成：下载二进制 + sha256 校验 + 原子替换 + 环境文件（token 不进
 * 进程列表）+ systemd 注册与重启。卸载同脚本 --uninstall，不触碰备份数据。
 */
export function agentInstallSnippet(
  token: string,
  loc: { protocol: string; host: string },
  arch: "amd64" | "arm64" = "amd64",
): string {
  const secure = loc.protocol === "https:";
  const server = `${secure ? "wss" : "ws"}://${loc.host}`;
  const base = "https://github.com/SinGyoZ/TarShip/releases/latest/download";
  const args = [`--server ${server}`, `--token ${token}`, `--arch ${arch}`];
  if (!secure) {
    // 面板走 http 时 Agent 默认拒绝明文连接，安装命令附带调试开关
    args.push("--allow-insecure-ws");
  }
  return `curl -fsSL ${base}/install-agent.sh | sh -s -- ${args.join(" ")}`;
}

/** 卸载命令：停止服务并删除二进制与配置，不影响已产生的备份产物。 */
export function agentUninstallSnippet(): string {
  const base = "https://github.com/SinGyoZ/TarShip/releases/latest/download";
  return `curl -fsSL ${base}/install-agent.sh | sh -s -- --uninstall`;
}

/** http 面板的附注（生产环境应为 Hub 配置 HTTPS）。 */
export function insecureNote(loc: { protocol: string }): string | null {
  return loc.protocol === "https:" ? null : "当前面板使用 http，安装命令已附带明文连接调试开关；生产环境请为 Hub 配置 HTTPS。";
}
