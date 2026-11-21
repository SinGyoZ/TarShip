import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { PanelEvent, Server } from "../types";

export type SocketState = "connecting" | "open" | "closed";

/**
 * 连接 /ws/panel，把 Hub 推送的在线状态合并进 ["servers"] 缓存。
 * 断线后指数退避重连；重连成功后 Hub 会先发完整快照，所以不会丢状态。
 */
export function usePanelSocket(): SocketState {
  const qc = useQueryClient();
  const [state, setState] = useState<SocketState>("connecting");

  useEffect(() => {
    let ws: WebSocket | null = null;
    let timer: number | undefined;
    let attempt = 0;
    let stopped = false;

    const apply = (ev: PanelEvent) => {
      switch (ev.type) {
        case "snapshot":
          qc.setQueryData<Server[]>(["servers"], ev.servers);
          break;
        case "server_status":
          qc.setQueryData<Server[]>(["servers"], (old) =>
            old?.map((s) =>
              s.id === ev.server_id
                ? {
                    ...s,
                    online: ev.online,
                    last_seen_at: ev.last_seen_at,
                    hostname: ev.hostname ?? s.hostname,
                    agent_version: ev.agent_version ?? s.agent_version,
                  }
                : s,
            ),
          );
          // 版本落后统计依赖在线状态与 Agent 版本，一并失效
          void qc.invalidateQueries({ queryKey: ["dashboard"] });
          break;
        case "servers_changed":
          void qc.invalidateQueries({ queryKey: ["servers"] });
          void qc.invalidateQueries({ queryKey: ["dashboard"] });
          break;
        case "run_update":
        case "runs_changed":
          // 备份进度与结果：失效运行历史与产物登记
          void qc.invalidateQueries({ queryKey: ["runs"] });
          void qc.invalidateQueries({ queryKey: ["backups"] });
          void qc.invalidateQueries({ queryKey: ["dashboard"] });
          break;
        case "tasks_changed":
          void qc.invalidateQueries({ queryKey: ["tasks"] });
          void qc.invalidateQueries({ queryKey: ["runs"] });
          void qc.invalidateQueries({ queryKey: ["backups"] });
          break;
      }
    };

    const connect = () => {
      setState("connecting");
      const scheme = location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(`${scheme}//${location.host}/ws/panel`);
      ws.onopen = () => {
        attempt = 0;
        setState("open");
      };
      ws.onmessage = (msg) => {
        try {
          apply(JSON.parse(msg.data as string) as PanelEvent);
        } catch {
          // 忽略无法解析的消息
        }
      };
      ws.onclose = () => {
        if (stopped) return;
        setState("closed");
        // 断线可能是会话过期：重新校验登录状态，失效时路由守卫会跳转到登录页。
        void qc.invalidateQueries({ queryKey: ["me"] });
        timer = window.setTimeout(connect, Math.min(30_000, 1000 * 2 ** attempt++));
      };
    };

    connect();
    return () => {
      stopped = true;
      window.clearTimeout(timer);
      ws?.close();
    };
  }, [qc]);

  return state;
}
