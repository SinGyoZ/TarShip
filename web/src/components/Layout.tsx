import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Button, Chip } from '@heroui/react';
import { api, type Me } from '../api';
import { usePanelSocket } from '../ws';

const NAV_ITEMS = [
  { to: '/', label: '仪表盘', end: true },
  { to: '/servers', label: '服务器' },
  { to: '/tasks', label: '任务' },
  { to: '/backups', label: '备份库' },
  { to: '/storages', label: '存储器' },
  { to: '/settings', label: '设置' },
];

export default function Layout() {
  usePanelSocket();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const logout = useMutation({
    mutationFn: () => api<{ ok: boolean }>('/api/auth/logout', { method: 'POST' }),
    onSuccess: () => {
      queryClient.clear();
      navigate('/login');
    },
  });

  const me = useQueryClient().getQueryData<Me>(['me']);

  return (
    <div className="min-h-screen bg-background text-foreground-600">
      <div className="mx-auto flex max-w-6xl gap-6 p-4 md:p-6">
        {/* 侧边导航：窄屏折叠为顶部横向滚动条，保证移动端可用 */}
        <nav className="flex shrink-0 flex-col gap-2 md:sticky md:top-6 md:h-fit md:w-44">
          <div className="mb-2 hidden items-center gap-2 px-2 md:flex">
            <span aria-hidden className="text-2xl">🚢</span>
            <div>
              <p className="text-lg font-bold text-foreground">TarShip</p>
              <p className="text-xs text-foreground-500">目录备份运输系统</p>
            </div>
          </div>
          <div className="flex gap-2 overflow-x-auto md:flex-col md:overflow-visible">
            {NAV_ITEMS.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  `whitespace-nowrap rounded-lg px-3 py-2 text-sm transition-colors ${
                    isActive
                      ? 'bg-primary/15 font-medium text-primary-600'
                      : 'text-foreground-500 hover:bg-default-100'
                  }`
                }
              >
                {item.label}
              </NavLink>
            ))}
          </div>
        </nav>

        <main className="min-w-0 flex-1">
          <header className="mb-4 flex items-center justify-between">
            <Chip variant="flat" color="default" size="sm">
              M0 骨架版 · {me?.version ?? ''}
            </Chip>
            <div className="flex items-center gap-3">
              <span className="hidden text-sm text-foreground-500 sm:block">{me?.username}</span>
              <Button
                size="sm"
                variant="flat"
                color="danger"
                isLoading={logout.isPending}
                onPress={() => logout.mutate()}
              >
                退出登录
              </Button>
            </div>
          </header>
          <Outlet />
        </main>
      </div>
    </div>
  );
}
