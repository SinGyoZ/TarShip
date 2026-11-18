import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

// 面板 WebSocket：订阅 Hub 推送的服务器状态事件。
// 收到事件后失效 servers / dashboard 查询缓存，由 TanStack Query 统一重新拉取，
// 避免本地维护在线状态导致与 REST 数据不一致。
export function usePanelSocket() {
  const queryClient = useQueryClient();

  useEffect(() => {
    let disposed = false;
    let retryTimer: number | undefined;

    // HttpOnly Cookie 会随同源 WS 请求自动携带，无需手动附加凭证
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${location.host}/ws/panel`);

    ws.onmessage = (e) => {
      let msg: { type?: string };
      try {
        msg = JSON.parse(e.data);
      } catch {
        return;
      }
      switch (msg.type) {
        case 'panel_hello':
        case 'server_online':
        case 'server_offline':
        case 'server_removed':
          void queryClient.invalidateQueries({ queryKey: ['servers'] });
          void queryClient.invalidateQueries({ queryKey: ['dashboard'] });
          break;
        default:
          break;
      }
    };
    ws.onclose = () => {
      if (disposed) return;
      // 断线后定时刷新页面重建连接；期间 REST 数据仍然有效，仅实时性受损
      retryTimer = window.setTimeout(() => window.location.reload(), 10_000);
    };

    return () => {
      disposed = true;
      if (retryTimer !== undefined) clearTimeout(retryTimer);
      ws.close();
    };
  }, [queryClient]);
}
