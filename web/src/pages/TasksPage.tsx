import { Button, Chip, Spinner } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorMessage } from "../api";
import { ConfirmModal } from "../components/ConfirmModal";
import { PageHeader } from "../components/PageHeader";
import { StatusDot } from "../components/StatusDot";
import { TaskFormModal } from "../components/TaskFormModal";
import { absoluteTime, relativeTime } from "../lib/format";
import { useNow } from "../hooks/useNow";
import type { Run, Server, Task } from "../types";

function formatBytes(n?: number): string {
  if (!n || n <= 0) return "-";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

export default function TasksPage() {
  const qc = useQueryClient();
  const now = useNow(30_000);
  const tasks = useQuery({ queryKey: ["tasks"], queryFn: api.listTasks });
  const servers = useQuery({ queryKey: ["servers"], queryFn: api.listServers });
  const storages = useQuery({ queryKey: ["storages"], queryFn: api.listStorages });
  const runs = useQuery({ queryKey: ["runs"], queryFn: api.listRuns, refetchInterval: 30_000 });
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Task | null>(null);
  const [toDelete, setToDelete] = useState<Task | null>(null);

  const serverByID = new Map<number, Server>((servers.data ?? []).map((s) => [s.id, s]));
  const storageName = new Map<number, string>((storages.data ?? []).map((s) => [s.id, s.name]));
  // 每个任务最近一次运行（runs 按时间倒序）
  const latestRunByTask = new Map<number, Run>();
  for (const rn of runs.data ?? []) {
    if (!latestRunByTask.has(rn.task_id)) latestRunByTask.set(rn.task_id, rn);
  }

  // 触发反馈由「运行中」徽章实时承载（run_update 事件驱动），失败原因行内展示
  const [triggerError, setTriggerError] = useState<string | null>(null);
  const runTask = useMutation({
    mutationFn: (id: number) => api.runTask(id),
    onSuccess: () => {
      setTriggerError(null);
      void qc.invalidateQueries({ queryKey: ["runs"] });
      void qc.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (e: Error) => setTriggerError(errorMessage(e)),
  });

  const list = tasks.data ?? [];

  const runBadge = (task: Task) => {
    const rn = latestRunByTask.get(task.id);
    if (!rn) return null;
    if (rn.status === "running") {
      return (
        <Chip size="sm" variant="soft">
          {`运行中 ${formatBytes(rn.uploaded_bytes)}`}
        </Chip>
      );
    }
    if (rn.status === "success") {
      return (
        <Chip size="sm" variant="soft" className="text-success">
          <time dateTime={rn.finished_at} title={absoluteTime(rn.finished_at ?? "")}>
            {`上次成功 ${relativeTime(rn.finished_at ?? "", now)} · ${formatBytes(rn.size_bytes)}`}
          </time>
        </Chip>
      );
    }
    return (
      <Chip size="sm" variant="soft" className="text-danger" title={rn.error_msg}>
        {`上次失败 ${relativeTime(rn.finished_at ?? "", now)}`}
      </Chip>
    );
  };

  return (
    <>
      <PageHeader
        title="备份任务"
        description="任务定义备份哪些目录、在哪台服务器上执行、产物存到哪个存储器。调度（cron）将在 M2 交付，当前为手动触发。"
        actions={<Button onPress={() => setFormOpen(true)}>新建任务</Button>}
      />

      {triggerError ? (
        <p role="alert" className="mb-3 rounded-lg border border-danger/40 px-3 py-2 text-sm text-danger">
          触发失败：{triggerError}
        </p>
      ) : null}

      {tasks.isPending ? (
        <div className="flex items-center gap-3 py-10 text-muted" role="status">
          <Spinner size="sm" />
          <span>正在加载任务…</span>
        </div>
      ) : tasks.isError ? (
        <div role="alert">
          <p className="font-medium">加载失败</p>
          <p className="mt-1 text-sm text-muted">{errorMessage(tasks.error)}</p>
        </div>
      ) : list.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-12 text-center">
          <p className="font-medium">还没有备份任务</p>
          <p className="mx-auto mt-1 max-w-sm text-sm text-muted">
            新建一个任务：选定服务器与目录、绑定存储器，然后手动触发第一次备份。
          </p>
          <Button className="mt-4" onPress={() => setFormOpen(true)}>
            新建任务
          </Button>
        </div>
      ) : (
        <ul className="divide-y divide-border rounded-xl border border-border bg-surface">
          {list.map((tk) => {
            const server = serverByID.get(tk.server_id);
            const running = latestRunByTask.get(tk.id)?.status === "running";
            return (
              <li key={tk.id} className="grid gap-3 px-4 py-4 md:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_auto] md:items-center">
                <div className="min-w-0">
                  <p className="truncate font-medium">{tk.name}</p>
                  <p className="mt-0.5 truncate font-mono text-xs text-muted" title={tk.paths.join("\n")}>
                    {tk.paths.join("  ")}
                  </p>
                  <div className="mt-1.5 flex flex-wrap items-center gap-2 text-xs text-muted">
                    {server ? (
                      <span className="flex items-center gap-1">
                        <StatusDot online={server.online} />
                        {server.name}
                      </span>
                    ) : (
                      <span className="text-danger">服务器已删除</span>
                    )}
                    <span>→ {storageName.get(tk.storage_id) ?? "存储器已删除"}</span>
                    {tk.excludes.length > 0 ? <span>{`排除 ${tk.excludes.length} 项`}</span> : null}
                  </div>
                </div>
                <div className="min-w-0">{runBadge(tk)}</div>
                <div className="flex gap-1">
                  <Button
                    size="sm"
                    isPending={runTask.isPending && runTask.variables === tk.id}
                    isDisabled={running || (server && !server.online)}
                    onPress={() => runTask.mutate(tk.id)}
                  >
                    {running ? "运行中" : "立即备份"}
                  </Button>
                  <Button size="sm" variant="ghost" onPress={() => setEditing(tk)}>
                    编辑
                  </Button>
                  <Button size="sm" variant="danger-soft" onPress={() => setToDelete(tk)}>
                    删除
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      {formOpen || editing ? (
        <TaskFormModal
          editing={editing}
          servers={servers.data ?? []}
          storages={storages.data ?? []}
          onClose={() => {
            setFormOpen(false);
            setEditing(null);
          }}
          onSaved={() => {
            setFormOpen(false);
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ["tasks"] });
          }}
        />
      ) : null}

      {toDelete ? (
        <ConfirmModal
          title={`删除任务「${toDelete.name}」`}
          body="运行历史与已产生的备份产物会一并删除登记，但不会删除存储器里的产物文件。此操作无法撤销。"
          confirmLabel="删除任务"
          danger
          onClose={() => setToDelete(null)}
          onConfirm={async () => {
            await api.deleteTask(toDelete.id);
            void qc.invalidateQueries({ queryKey: ["tasks"] });
            void qc.invalidateQueries({ queryKey: ["backups"] });
          }}
        />
      ) : null}
    </>
  );
}
