// API 调用封装：统一错误提取与 JSON 解析。
export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    ...options,
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    credentials: 'same-origin',
  });
  const body = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    const err = (body as { error?: string }).error || `请求失败（HTTP ${resp.status}）`;
    throw new Error(err);
  }
  return body as T;
}

export interface AuthStatus {
  initialized: boolean;
  version: string;
}

export interface Me {
  username: string;
  version: string;
}

export interface ServerRow {
  id: number;
  name: string;
  online: boolean;
  hostname: string;
  agent_version?: string;
  tags: string;
  last_seen: string;
  created_at: string;
}

export interface CreateServerResult {
  id: number;
  name: string;
  token: string;
  install_command: string;
}

export interface DashboardStats {
  version: string;
  servers: { total: number; online: number; offline: number; outdated: number };
}
