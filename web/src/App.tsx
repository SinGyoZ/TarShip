import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { Spinner } from '@heroui/react';
import Layout from './components/Layout';
import LoginPage from './pages/LoginPage';
import DashboardPage from './pages/DashboardPage';
import ServersPage from './pages/ServersPage';
import PlaceholderPage from './pages/PlaceholderPage';
import { api, type Me } from './api';

// 登录态判定：/api/me 200 即已登录；401 由 api 层抛错。
function useSession() {
  return useQuery({
    queryKey: ['me'],
    queryFn: () => api<Me>('/api/me'),
    retry: false,
  });
}

function RequireAuth() {
  const location = useLocation();
  const { isPending, isError } = useSession();
  if (isPending) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Spinner label="加载中…" />
      </div>
    );
  }
  if (isError) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  return <Outlet />;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<RequireAuth />}>
        <Route element={<Layout />}>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/servers" element={<ServersPage />} />
          <Route
            path="/tasks"
            element={
              <PlaceholderPage
                title="任务"
                milestone="M1"
                description="任务定义备份的目录、排除规则、调度与保留策略。任务管理将在 M1（备份闭环）交付。"
              />
            }
          />
          <Route
            path="/backups"
            element={
              <PlaceholderPage
                title="备份库"
                milestone="M1"
                description="全部 .tar.zst 产物列表与恢复入口。备份列表将在 M1（备份闭环）交付，一键恢复在 M3。"
              />
            }
          />
          <Route
            path="/storages"
            element={
              <PlaceholderPage
                title="存储器"
                milestone="M1"
                description="S3 兼容与本地存储配置、连通性测试。存储管理将在 M1（备份闭环）交付。"
              />
            }
          />
          <Route
            path="/settings"
            element={
              <PlaceholderPage
                title="设置"
                milestone="M2"
                description="面板账号、webhook 通知与回收站策略将在 M2（调度自动化）起陆续交付。"
              />
            }
          />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
