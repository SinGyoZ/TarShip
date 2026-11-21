export interface Server {
  id: number;
  name: string;
  hostname: string;
  agent_version: string;
  tags: string[];
  online: boolean;
  last_seen_at: string | null;
  created_at: string;
}

export interface ServerWithToken {
  server: Server;
  token: string;
}

/** /api/dashboard 聚合统计。 */
export interface DashboardStats {
  version: string;
  servers: { total: number; online: number; offline: number; outdated: number };
}

/** /ws/panel 推送的事件。 */
export type PanelEvent =
  | { type: "snapshot"; servers: Server[] }
  | {
      type: "server_status";
      server_id: number;
      online: boolean;
      hostname?: string;
      agent_version?: string;
      last_seen_at: string;
    }
  | { type: "servers_changed" };
