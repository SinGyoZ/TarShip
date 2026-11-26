#!/bin/sh
# TarShip Agent 一键安装 / 卸载脚本（随 Release 发布，资产名固定）。
#
# 安装：install-agent.sh --server wss://hub.example.com --token tsa_xxx [--arch amd64|arm64] [--allow-insecure-ws]
# 卸载：install-agent.sh --uninstall
#
# 面板生成的推荐用法（token 会进入 shell history，属一次性安装场景可接受；泄露可在面板重置）：
#   curl -fsSL https://github.com/SinGyoZ/TarShip/releases/latest/download/install-agent.sh | sh -s -- --server ... --token ...
set -eu

RELEASE_BASE="https://github.com/SinGyoZ/TarShip/releases/latest/download"
BIN_PATH="/usr/local/bin/tarship"
ENV_PATH="/etc/tarship-agent.env"
UNIT_PATH="/etc/systemd/system/tarship-agent.service"

usage() {
  cat <<'EOF'
用法:
  安装  install-agent.sh --server wss://hub.example.com --token <TOKEN> [选项]
  卸载  install-agent.sh --uninstall

选项:
  --arch amd64|arm64     目标机器架构，默认 amd64
  --allow-insecure-ws    允许明文 ws:// 连接（仅本地调试）
  --uninstall            卸载 Agent（停止服务并删除二进制与配置，不影响已产生的备份数据）
EOF
  exit 2
}

die() { echo "安装失败: $*" >&2; exit 1; }

# 需要写 /usr/local/bin 与 /etc，统一要求 root，避免 sudo 判断的分叉。
[ "$(id -u)" = "0" ] || die "请以 root 运行（或加 sudo）"

uninstall() {
  echo "正在卸载 TarShip Agent…"
  systemctl disable --now tarship-agent 2>/dev/null || true
  rm -f "$UNIT_PATH" "$ENV_PATH" "$BIN_PATH"
  systemctl daemon-reload
  echo "已卸载：服务已停止，二进制与配置已删除。"
  echo "已产生的备份数据未做改动（存储目录/对象存储中的 .tar.zst 仍保留）。"
}

install_agent() {
  echo "==> 下载 tarship-linux-${ARCH} 并校验 sha256…"
  curl -fsSL "${RELEASE_BASE}/tarship-linux-${ARCH}" -o /tmp/tarship.new \
    || die "下载二进制失败（检查网络与架构 ${ARCH}）"
  curl -fsSL "${RELEASE_BASE}/checksums.txt" -o /tmp/tarship.sha256 \
    || die "下载校验和失败"
  expected=$(grep "tarship-linux-${ARCH}" /tmp/tarship.sha256 | awk '{print $1}')
  [ -n "$expected" ] || die "校验和文件中没有 ${ARCH} 条目"
  # sha256sum 校验临时文件名，比对提取的哈希，防传输损坏与劫持
  echo "${expected}  tarship.new" | (cd /tmp && sha256sum -c -) >/dev/null \
    || die "sha256 校验不通过，拒绝安装"
  chmod +x /tmp/tarship.new
  # 覆盖运行中的二进制必须走 mv 原子替换，直接 -o 会触发 ETXTBSY
  mv /tmp/tarship.new "$BIN_PATH"

  echo "==> 写入配置 ${ENV_PATH}（权限 600，token 不进进程列表）…"
  umask 077
  cat > "$ENV_PATH" <<EOF
TARSHIP_SERVER=${SERVER}
TARSHIP_TOKEN=${TOKEN}
EOF
  [ "$ALLOW_INSECURE" = "1" ] && printf 'TARSHIP_ALLOW_INSECURE_WS=true\n' >> "$ENV_PATH"
  chmod 600 "$ENV_PATH"

  echo "==> 注册 systemd 服务…"
  cat > "$UNIT_PATH" <<'EOF'
[Unit]
Description=TarShip Agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/tarship-agent.env
ExecStart=/usr/local/bin/tarship agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  # enable 幂等；restart 保证全新安装与升级重装都生效
  systemctl enable tarship-agent >/dev/null
  systemctl restart tarship-agent

  echo
  echo "安装完成。查看状态: systemctl status tarship-agent"
  echo "排查日志:     journalctl -u tarship-agent -f"
}

SERVER="" TOKEN="" ARCH="amd64" ALLOW_INSECURE=0
while [ $# -gt 0 ]; do
  case "$1" in
    --server) SERVER="${2:?}"; shift 2 ;;
    --token) TOKEN="${2:?}"; shift 2 ;;
    --arch) ARCH="${2:?}"; shift 2 ;;
    --allow-insecure-ws) ALLOW_INSECURE=1; shift ;;
    --uninstall) uninstall; exit 0 ;;
    -h|--help) usage ;;
    *) echo "未知参数: $1" >&2; usage ;;
  esac
done

[ "$ARCH" = "amd64" ] || [ "$ARCH" = "arm64" ] || die "--arch 仅支持 amd64 / arm64"
[ -n "$SERVER" ] || { echo "缺少 --server" >&2; usage; }
[ -n "$TOKEN" ] || { echo "缺少 --token" >&2; usage; }
# 明文连接由脚本显式声明，避免用户没意识到 token 会明文传输
case "$SERVER" in
  ws://*) [ "$ALLOW_INSECURE" = "1" ] || die "拒绝明文 ws://（token 会明文传输）；仅本地调试请加 --allow-insecure-ws" ;;
  wss://*) ;;
  *) die "--server 必须以 ws:// 或 wss:// 开头" ;;
esac

install_agent
