import { Button, Chip, Spinner } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api, errorMessage } from "../api";
import { PageHeader } from "../components/PageHeader";
import { StatusDot } from "../components/StatusDot";
import { useNow } from "../hooks/useNow";
import { relativeTime } from "../lib/format";
import type { Run } from "../types";

function StatCard({ label, value, tone }: { label: string; value: number; tone?: "ok" | "warn" | "danger" }) {
  const toneClass =
    tone === "danger" ? "text-danger" : tone === "warn" ? "text-warning" : tone === "ok" ? "text-success" : "";
  return (
    <div className="rounded-xl border border-border bg-surface px-4 py-3">
      <p className={`text-2xl font-semibold ${toneClass}`}>{value}</p>
      <p className="mt-1 text-sm text-muted">{label}</p>
    </div>
  );
}

export default function DashboardPage() {
  const servers = useQuery({ queryKey: ["servers"], queryFn: api.listServers });
  const stats = useQuery({ queryKey: ["dashboard"], queryFn: api.dashboard });
  const runs = useQuery({ queryKey: ["runs"], queryFn: api.listRuns });
  const now = useNow(30_000);

  if (servers.isPending) {
    return (
      <div className="flex items-center gap-3 py-10 text-muted" role="status">
        <Spinner size="sm" />
        <span>正在加载…</span>
      </div>
    );
  }
  if (servers.isError) {
    return (
      <div role="alert">
        <p className="font-medium">加载失败</p>
        <p className="mt-1 text-sm text-muted">{errorMessage(servers.error)}</p>
        <Button className="mt-4" variant="secondary" onPress={() => void servers.refetch()}>
          重试
        </Button>
      </div>
    );
  }

  const list = servers.data;
  const offline = list.filter((s) => !s.online);
  const outdated = stats.data?.servers.outdated ?? 0;

  if (list.length === 0) {
    return (
      <>
        <PageHeader title="仪表盘" />
        <div className="max-w-md">
          <p className="font-medium">先添加一台服务器</p>
          <p className="mt-1 text-sm text-muted">让目标机器上的 Agent 连上 Hub，在线状态会实时显示在这里。</p>
          <Link to="/servers" className="mt-4 inline-block rounded-lg bg-accent px-4 py-2 text-sm font-medium text-accent-foreground">
            添加服务器
          </Link>
        </div>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="仪表盘"
        description={stats.data ? `Hub 版本 ${stats.data.version}` : undefined}
      />
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard label="服务器总数" value={stats.data?.servers.total ?? list.length} />
        <StatCard label="在线" value={stats.data?.servers.online ?? list.length - offline.length} tone="ok" />
        <StatCard label="离线" value={stats.data?.servers.offline ?? offline.length} tone={offline.length > 0 ? "warn" : undefined} />
        <StatCard label="版本落后" value={outdated} tone={outdated > 0 ? "danger" : undefined} />
      </div>

      {outdated > 0 ? (
        <p role="status" className="mt-3 text-sm text-warning">
          有 {outdated} 台 Agent 版本低于 Hub，请到「服务器」页重置对应 token 后重新执行安装命令完成升级。
        </p>
      ) : null}

      {offline.length > 0 ? (
        <section aria-labelledby="offline-heading" className="mt-6">
          <h2 id="offline-heading" className="mb-2 text-base font-semibold">
            需要关注：{offline.length} 台离线
          </h2>
          <ul className="divide-y divide-border rounded-xl border border-border bg-surface">
            {offline.map((s) => (
              <li key={s.id} className="flex flex-wrap items-center justify-between gap-2 px-4 py-3">
                <span className="font-medium">{s.name}</span>
                <span className="flex items-center gap-4 text-sm text-muted">
                  <StatusDot online={false} />
                  {s.last_seen_at ? `最后在线 ${relativeTime(s.last_seen_at, now)}` : "从未连接"}
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : (
        <p className="mt-6 text-sm">所有服务器都在线。</p>
      )}

      <section aria-labelledby="recent-runs-heading" className="mt-8">
        <div className="mb-2 flex items-center justify-between">
          <h2 id="recent-runs-heading" className="text-base font-semibold">
            最近备份
          </h2>
          <Link to="/tasks" className="text-sm text-accent hover:underline">
            去任务页
          </Link>
        </div>
        {runs.isError ? (
          <p role="alert" className="text-sm text-danger">
            {errorMessage(runs.error)}
          </p>
        ) : (runs.data ?? []).length === 0 ? (
          <p className="text-sm text-muted">还没有备份记录，到「任务」页新建任务并触发第一次备份。</p>
        ) : (
          <ul className="divide-y divide-border rounded-xl border border-border bg-surface">
            {(runs.data ?? []).slice(0, 5).map((rn: Run) => (
              <li key={rn.id} className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-sm">
                <span className="flex items-center gap-2">
                  {rn.status === "success" ? (
                    <Chip size="sm" variant="soft" className="text-success">
                      成功
                    </Chip>
                  ) : rn.status === "running" ? (
                    <Chip size="sm" variant="soft">
                      运行中
                    </Chip>
                  ) : (
                    <Chip size="sm" variant="soft" className="text-danger">
                      失败
                    </Chip>
                  )}
                  <span>{`任务 #${rn.task_id}`}</span>
                </span>
                <span className="text-xs text-muted" title={rn.error_msg ?? ""}>
                  {relativeTime(rn.started_at, now)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  );
}
