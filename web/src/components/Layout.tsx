import { Button, cn } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { api } from "../api";
import { usePanelSocket, type SocketState } from "../hooks/usePanelSocket";
import { Wordmark } from "./Wordmark";

const NAV = [
  { to: "/", label: "仪表盘", end: true },
  { to: "/servers", label: "服务器" },
  { to: "/tasks", label: "备份任务" },
  { to: "/backups", label: "备份库" },
  { to: "/storages", label: "存储器" },
  { to: "/settings", label: "设置" },
];

const SOCKET_TEXT: Record<SocketState, string> = {
  open: "实时更新已连接",
  connecting: "正在连接实时更新…",
  closed: "实时更新已断开，正在重试",
};

export default function Layout() {
  const socket = usePanelSocket();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, staleTime: 30_000 });
  const logout = useMutation({
    mutationFn: api.logout,
    onSettled: () => {
      qc.setQueryData(["me"], null);
      qc.removeQueries({ queryKey: ["servers"] });
      navigate("/login", { replace: true });
    },
  });

  return (
    <div className="min-h-dvh md:grid md:grid-cols-[13rem_minmax(0,1fr)]">
      <aside className="border-b border-border bg-surface md:sticky md:top-0 md:flex md:h-dvh md:flex-col md:border-r md:border-b-0">
        <div className="flex items-center justify-between px-4 py-3 md:px-5 md:py-5">
          <Wordmark />
          <Button size="sm" variant="ghost" className="md:hidden" onPress={() => logout.mutate()}>
            退出
          </Button>
        </div>
        <nav aria-label="主导航" className="flex gap-1 overflow-x-auto px-2 pb-2 md:flex-1 md:flex-col md:overflow-visible md:px-3">
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  "shrink-0 rounded-lg px-3 py-2 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-focus",
                  isActive ? "bg-accent text-accent-foreground font-medium" : "text-foreground hover:bg-surface-secondary",
                )
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="hidden space-y-2 border-t border-border p-4 text-sm md:block">
          <p className="flex items-center gap-2 text-xs text-muted" role="status">
            <span aria-hidden="true" className={cn("size-2 rounded-full", socket === "open" ? "bg-success" : "bg-warning")} />
            {SOCKET_TEXT[socket]}
          </p>
          <div className="flex items-center justify-between gap-2">
            <span className="truncate font-medium">{me.data?.username}</span>
            <Button size="sm" variant="ghost" onPress={() => logout.mutate()}>
              退出登录
            </Button>
          </div>
        </div>
      </aside>
      <main className="min-w-0 px-4 py-6 md:px-10 md:py-8">
        <Outlet />
      </main>
    </div>
  );
}
