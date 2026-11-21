import { Chip, Spinner } from "@heroui/react";
import { useQuery } from "@tanstack/react-query";
import { api, errorMessage } from "../api";
import { PageHeader } from "../components/PageHeader";
import { useNow } from "../hooks/useNow";
import { absoluteTime, relativeTime } from "../lib/format";

function formatBytes(n?: number): string {
  if (!n || n <= 0) return "-";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

const statusChip = (status: string) => {
  if (status === "success") return <Chip size="sm" variant="soft" className="text-success">成功</Chip>;
  if (status === "running") return <Chip size="sm" variant="soft">运行中</Chip>;
  return <Chip size="sm" variant="soft" className="text-danger">失败</Chip>;
};

/** 备份库：运行历史 + 产物登记。恢复入口在 M3 交付。 */
export default function BackupsPage() {
  const now = useNow(30_000);
  const runs = useQuery({ queryKey: ["runs"], queryFn: api.listRuns });
  const backups = useQuery({ queryKey: ["backups"], queryFn: api.listBackups });
  const tasks = useQuery({ queryKey: ["tasks"], queryFn: api.listTasks });
  const servers = useQuery({ queryKey: ["servers"], queryFn: api.listServers });

  const taskName = new Map<number, string>((tasks.data ?? []).map((t) => [t.id, t.name]));
  const serverName = new Map<number, string>((servers.data ?? []).map((s) => [s.id, s.name]));

  return (
    <>
      <PageHeader
        title="备份库"
        description="全部备份产物与运行历史。产物为标准 .tar.zst，可用 sha256 校验、可脱离 TarShip 手工恢复。"
      />

      <section aria-labelledby="runs-heading">
        <h2 id="runs-heading" className="mb-2 text-base font-semibold">
          最近运行
        </h2>
        {runs.isPending ? (
          <div className="flex items-center gap-3 py-6 text-muted" role="status">
            <Spinner size="sm" />
            <span>正在加载…</span>
          </div>
        ) : runs.isError ? (
          <p role="alert" className="text-sm text-danger">
            {errorMessage(runs.error)}
          </p>
        ) : (runs.data ?? []).length === 0 ? (
          <p className="rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-muted">
            还没有运行记录，去「任务」页触发第一次备份。
          </p>
        ) : (
          <ul className="divide-y divide-border rounded-xl border border-border bg-surface">
            {(runs.data ?? []).map((rn) => (
              <li key={rn.id} className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-sm">
                <span className="flex flex-wrap items-center gap-2">
                  {statusChip(rn.status)}
                  <span className="font-medium">{taskName.get(rn.task_id) ?? `任务 #${rn.task_id}`}</span>
                  <span className="text-muted">{serverName.get(rn.server_id) ?? `服务器 #${rn.server_id}`}</span>
                  {rn.status === "running" && rn.uploaded_bytes ? (
                    <span className="text-muted">{`已写入 ${formatBytes(rn.uploaded_bytes)}`}</span>
                  ) : null}
                  {rn.size_bytes ? <span className="text-muted">{formatBytes(rn.size_bytes)}</span> : null}
                </span>
                <span className="text-xs text-muted" title={absoluteTime(rn.started_at)}>
                  {relativeTime(rn.started_at, now)}
                  {rn.error_msg ? ` · ${rn.error_msg}` : ""}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section aria-labelledby="backups-heading" className="mt-8">
        <h2 id="backups-heading" className="mb-2 text-base font-semibold">
          备份产物
        </h2>
        {backups.isPending ? (
          <div className="flex items-center gap-3 py-6 text-muted" role="status">
            <Spinner size="sm" />
            <span>正在加载…</span>
          </div>
        ) : backups.isError ? (
          <p role="alert" className="text-sm text-danger">
            {errorMessage(backups.error)}
          </p>
        ) : (backups.data ?? []).length === 0 ? (
          <p className="rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-muted">
            成功的备份产物会登记在这里。
          </p>
        ) : (
          <div className="overflow-x-auto rounded-xl border border-border bg-surface">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs text-muted">
                  <th className="px-4 py-2 font-medium">时间</th>
                  <th className="px-4 py-2 font-medium">任务 / 服务器</th>
                  <th className="px-4 py-2 font-medium">大小</th>
                  <th className="px-4 py-2 font-medium">sha256</th>
                  <th className="px-4 py-2 font-medium">对象键</th>
                </tr>
              </thead>
              <tbody>
                {(backups.data ?? []).map((b) => (
                  <tr key={b.id} className="border-b border-border last:border-0">
                    <td className="px-4 py-2 text-xs text-muted" title={absoluteTime(b.created_at)}>
                      {relativeTime(b.created_at, now)}
                    </td>
                    <td className="px-4 py-2">
                      <span className="font-medium">{taskName.get(b.task_id) ?? `任务 #${b.task_id}`}</span>
                      <span className="text-muted"> / {serverName.get(b.server_id) ?? "-"}</span>
                    </td>
                    <td className="px-4 py-2">{formatBytes(b.size_bytes)}</td>
                    <td className="px-4 py-2 font-mono text-xs" title={b.sha256}>
                      {`${b.sha256.slice(0, 12)}…`}
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-muted" title={b.object_key}>
                      {b.object_key}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}
