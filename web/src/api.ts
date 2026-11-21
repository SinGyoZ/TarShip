import type { DashboardStats, Server, ServerWithToken } from "./types";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let resp: Response;
  try {
    resp = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, "network", "无法连接到 Hub，请检查网络后重试");
  }
  if (resp.status === 204) {
    return undefined as T;
  }
  const data: unknown = await resp.json().catch(() => null);
  if (!resp.ok) {
    const err = (data as { error?: { code?: string; message?: string } } | null)?.error;
    throw new ApiError(resp.status, err?.code ?? "unknown", err?.message ?? `请求失败（HTTP ${resp.status}）`);
  }
  return data as T;
}

export const api = {
  setupStatus: () => request<{ initialized: boolean }>("GET", "/api/setup/status"),
  setup: (body: { setup_token: string; username: string; password: string }) =>
    request<{ username: string }>("POST", "/api/auth/setup", body),
  login: (body: { username: string; password: string }) =>
    request<{ username: string }>("POST", "/api/auth/login", body),
  logout: () => request<void>("POST", "/api/auth/logout"),
  /** 未登录时返回 null，而不是抛错，方便路由守卫判断。 */
  me: async (): Promise<{ username: string } | null> => {
    try {
      return await request<{ username: string }>("GET", "/api/auth/me");
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return null;
      throw e;
    }
  },
  listServers: () => request<Server[]>("GET", "/api/servers"),
  dashboard: () => request<DashboardStats>("GET", "/api/dashboard"),
  createServer: (name: string) => request<ServerWithToken>("POST", "/api/servers", { name }),
  deleteServer: (id: number) => request<void>("DELETE", `/api/servers/${id}`),
  resetToken: (id: number) => request<ServerWithToken>("POST", `/api/servers/${id}/reset-token`),
};

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : "发生未知错误";
}
