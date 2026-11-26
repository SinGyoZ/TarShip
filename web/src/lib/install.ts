/**
 * Agent 安装/卸载命令生成。
 *
 * 生成的命令是自包含的一条复合命令（&& 串联）：复制一次、粘贴一次、
 * 回车执行完成全部步骤，任何一步失败立即中止。不依赖任何外部脚本。
 *
 * 内置的经验修复（全部来自实测踩坑）：
 * - 下载后用 Release 校验和验证 sha256 再落地，防传输损坏与劫持；
 * - 覆盖运行中的二进制必须走 /tmp + mv 原子替换（直接 -o 会触发 ETXTBSY）；
 * - token 写进 600 权限环境文件，由 systemd 注入，不出现在进程列表；
 * - 结尾 daemon-reload + enable + restart（enable --now 对已运行服务不会重启，
 *   升级重装时新二进制不生效）。
 */

const RELEASE_BASE = "https://github.com/SinGyoZ/TarShip/releases/latest/download";

export function agentInstallSnippet(
  token: string,
  loc: { protocol: string; host: string },
  arch: "amd64" | "arm64" = "amd64",
): string {
  const secure = loc.protocol === "https:";
  const server = `${secure ? "wss" : "ws"}://${loc.host}`;

  const envLines = [`TARSHIP_SERVER=${server}`, `TARSHIP_TOKEN=${token}`];
  if (!secure) {
    envLines.push("TARSHIP_ALLOW_INSECURE_WS=true"); // 面板走 http 时 Agent 默认拒绝明文
  }
  const envBlock = envLines.map((l) => `'${l}'`).join(" ");

  const unitBlock = [
    "[Unit]",
    "Description=TarShip Agent",
    "After=network-online.target",
    "Wants=network-online.target",
    "",
    "[Service]",
    "EnvironmentFile=/etc/tarship-agent.env",
    "ExecStart=/usr/local/bin/tarship agent",
    "Restart=always",
    "RestartSec=5",
    "",
    "[Install]",
    "WantedBy=multi-user.target",
  ]
    .map((l) => `'${l}'`)
    .join(" ");

  // 单条复合命令：任一步失败即中止，不留半成品
  return [
    `curl -fsSL ${RELEASE_BASE}/tarship-linux-${arch} -o /tmp/tarship.new \\`,
    `  && curl -fsSL ${RELEASE_BASE}/checksums.txt -o /tmp/tarship.sha256 \\`,
    `  && grep "tarship-linux-${arch}" /tmp/tarship.sha256 | awk '{print $1"  tarship.new"}' | (cd /tmp && sha256sum -c -) \\`,
    `  && sudo chmod +x /tmp/tarship.new \\`,
    `  && sudo mv /tmp/tarship.new /usr/local/bin/tarship \\`,
    `  && sudo install -m 600 /dev/null /etc/tarship-agent.env \\`,
    `  && printf '%s\\n' ${envBlock} | sudo tee /etc/tarship-agent.env >/dev/null \\`,
    `  && printf '%s\\n' ${unitBlock} | sudo tee /etc/systemd/system/tarship-agent.service >/dev/null \\`,
    `  && sudo systemctl daemon-reload \\`,
    `  && sudo systemctl enable tarship-agent >/dev/null \\`,
    `  && sudo systemctl restart tarship-agent \\`,
    `  && echo '安装完成：systemctl status tarship-agent 查看状态，journalctl -u tarship-agent -f 看日志'`,
  ].join("\n");
}

/** 卸载：停服务、删文件、清 systemd；已产生的备份数据不受影响。首段容错（服务可能未装）。 */
export function agentUninstallSnippet(): string {
  return [
    "sudo systemctl disable --now tarship-agent 2>/dev/null; \\",
    "  sudo rm -f /etc/systemd/system/tarship-agent.service /etc/tarship-agent.env /usr/local/bin/tarship \\",
    "  && sudo systemctl daemon-reload \\",
    "  && echo '已卸载：服务与配置已删除，已产生的备份数据不受影响'",
  ].join("\n");
}

/** http 面板的附注（生产环境应为 Hub 配置 HTTPS）。 */
export function insecureNote(loc: { protocol: string }): string | null {
  return loc.protocol === "https:" ? null : "当前面板使用 http，安装命令已附带明文连接调试开关；生产环境请为 Hub 配置 HTTPS。";
}
