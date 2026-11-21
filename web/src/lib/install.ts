/**
 * 生成在目标服务器上安装并常驻运行 Agent 的一键命令。
 *
 * 融合三段经验：
 * - 下载走 /tmp 再 mv：直接 -o 覆盖正在运行的二进制会触发 ETXTBSY 写入保护（v0.1.4 修复）；
 * - token 写进权限 600 的环境文件，由 systemd 注入，不出现在进程列表里；
 * - 结尾 daemon-reload + enable + restart：enable --now 对已运行服务不会重启，升级重装时新二进制不生效（v0.1.3 修复）。
 */
export function agentInstallSnippet(
  token: string,
  loc: { protocol: string; host: string },
  arch: "amd64" | "arm64" = "amd64",
): string {
  const secure = loc.protocol === "https:";
  const server = `${secure ? "wss" : "ws"}://${loc.host}`;
  return [
    "# 1. 下载 tarship 到临时文件并原子替换（覆盖运行中的二进制必须走 mv，不能直接 -o）",
    `curl -fsSL https://github.com/SinGyoZ/TarShip/releases/latest/download/tarship-linux-${arch} -o /tmp/tarship.new \\`,
    "  && chmod +x /tmp/tarship.new \\",
    "  && mv /tmp/tarship.new /usr/local/bin/tarship",
    "",
    "# 2. 写入配置文件（权限 600，token 不出现在进程列表里）",
    ...(secure ? [] : ["# 当前面板使用 http，Agent 默认拒绝明文连接；下面加了调试开关，生产环境请为 Hub 配置 HTTPS"]),
    "sudo install -m 600 /dev/null /etc/tarship-agent.env",
    "sudo tee /etc/tarship-agent.env >/dev/null <<'EOF'",
    `TARSHIP_SERVER=${server}`,
    `TARSHIP_TOKEN=${token}`,
    ...(secure ? [] : ["TARSHIP_ALLOW_INSECURE_WS=true"]),
    "EOF",
    "",
    "# 3. 注册为 systemd 服务并启动（全新安装与升级重装同一套命令）",
    "sudo tee /etc/systemd/system/tarship-agent.service >/dev/null <<'EOF'",
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
    "EOF",
    "sudo systemctl daemon-reload && sudo systemctl enable tarship-agent && sudo systemctl restart tarship-agent",
  ].join("\n");
}
