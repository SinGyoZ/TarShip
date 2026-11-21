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

/** 存储器（secret 不回显）。 */
export interface Storage {
  id: number;
  name: string;
  type: "local" | "s3";
  root_dir?: string;
  endpoint?: string;
  region?: string;
  bucket?: string;
  access_key_id?: string;
  prefix?: string;
  created_at: string;
}

export interface StorageInput {
  name: string;
  type: "local" | "s3";
  root_dir?: string;
  endpoint?: string;
  region?: string;
  bucket?: string;
  access_key_id?: string;
  secret_access_key?: string;
  prefix?: string;
}

/** 备份任务。 */
export interface Task {
  id: number;
  name: string;
  server_id: number;
  storage_id: number;
  paths: string[];
  excludes: string[];
  cron_expr?: string;
  retention_days?: number;
  enabled: boolean;
  created_at: string;
}

export interface TaskInput {
  name: string;
  server_id: number;
  storage_id: number;
  paths: string[];
  excludes: string[];
}

/** 一次备份运行。 */
export interface Run {
  id: number;
  task_id: number;
  server_id: number;
  storage_id: number;
  status: "running" | "success" | "failed";
  size_bytes?: number;
  uploaded_bytes?: number;
  sha256?: string;
  object_key?: string;
  error_msg?: string;
  started_at: string;
  finished_at?: string;
}

/** 备份产物登记。 */
export interface Backup {
  id: number;
  run_id: number;
  task_id: number;
  server_id: number;
  object_key: string;
  size_bytes: number;
  sha256: string;
  created_at: string;
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
  | { type: "servers_changed" }
  | { type: "run_update"; run_id?: number; status?: string; uploaded_bytes?: number; run?: Run }
  | { type: "runs_changed" }
  | { type: "tasks_changed" };
