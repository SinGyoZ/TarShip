import { useQuery } from "@tanstack/react-query";
import { Button, Spinner } from "@heroui/react";
import { Navigate, Outlet } from "react-router-dom";
import { api, errorMessage } from "./api";

/** 同时查询「是否已初始化」与「是否已登录」，供路由守卫和登录/初始化页使用。 */
export function useBootstrap() {
  const setup = useQuery({ queryKey: ["setup"], queryFn: api.setupStatus, retry: false, staleTime: 30_000 });
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false, staleTime: 30_000 });
  return {
    loading: setup.isPending || me.isPending,
    error: setup.error ?? me.error,
    initialized: setup.data?.initialized ?? false,
    me: me.data ?? null,
    retry: () => {
      void setup.refetch();
      void me.refetch();
    },
  };
}

export function FullPageSpinner() {
  return (
    <div className="grid min-h-dvh place-items-center" role="status">
      <div className="flex items-center gap-3 text-muted">
        <Spinner size="sm" />
        <span>正在加载…</span>
      </div>
    </div>
  );
}

export function BootError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  return (
    <div className="grid min-h-dvh place-items-center px-4">
      <div className="max-w-sm text-center" role="alert">
        <p className="font-medium">无法连接到 Hub</p>
        <p className="mt-1 text-sm text-muted">{errorMessage(error)}</p>
        <Button className="mt-4" onPress={onRetry}>
          重试
        </Button>
      </div>
    </div>
  );
}

/** 路由守卫：未初始化去初始化页，未登录去登录页。 */
export function RequireAuth() {
  const boot = useBootstrap();
  if (boot.loading) return <FullPageSpinner />;
  if (boot.error) return <BootError error={boot.error} onRetry={boot.retry} />;
  if (!boot.initialized) return <Navigate to="/setup" replace />;
  if (!boot.me) return <Navigate to="/login" replace />;
  return <Outlet />;
}
