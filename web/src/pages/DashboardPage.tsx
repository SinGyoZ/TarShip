import { useQuery } from '@tanstack/react-query';
import { Card, CardBody, CardHeader, Chip } from '@heroui/react';
import { api, type DashboardStats } from '../api';

export default function DashboardPage() {
  const { data, isPending, isError, error } = useQuery({
    queryKey: ['dashboard'],
    queryFn: () => api<DashboardStats>('/api/dashboard'),
  });

  if (isPending) {
    return <p className="py-10 text-center text-sm text-foreground-500">加载中…</p>;
  }
  if (isError) {
    return <p className="py-10 text-center text-sm text-danger">统计加载失败：{error.message}</p>;
  }

  const { servers } = data;
  const stats = [
    { label: '服务器总数', value: servers.total },
    { label: '在线', value: servers.online, color: 'success' as const },
    { label: '离线', value: servers.offline, color: 'warning' as const },
    { label: '版本落后', value: servers.outdated, color: 'danger' as const },
  ];

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-xl font-bold">仪表盘</h1>
        <p className="mt-1 text-sm text-foreground-500">服务器在线概况与系统版本信息</p>
      </div>

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {stats.map((s) => (
          <Card key={s.label} shadow="sm">
            <CardBody className="items-start p-4">
              <p className="text-2xl font-bold">{s.value}</p>
              <div className="mt-1 flex items-center gap-2">
                <span className="text-xs text-foreground-500">{s.label}</span>
                {s.color && (
                  <Chip size="sm" variant="flat" color={s.color}>
                    {s.color === 'success' ? '正常' : s.color === 'warning' ? '待关注' : '需处理'}
                  </Chip>
                )}
              </div>
            </CardBody>
          </Card>
        ))}
      </div>

      <Card shadow="sm">
        <CardHeader className="flex flex-col items-start gap-1">
          <h2 className="text-base font-semibold">M0 骨架说明</h2>
        </CardHeader>
        <CardBody className="gap-2 text-sm text-foreground-500">
          <p>当前为 M0（骨架）版本，已交付：面板登录、服务器接入与实时在线状态。</p>
          <p>
            备份核心能力（存储器、任务、备份与恢复）将在 <strong>M1 起逐里程碑</strong>
            交付；Agent 与 Hub 版本不一致时会在服务器页提示升级。
          </p>
        </CardBody>
      </Card>
    </div>
  );
}
