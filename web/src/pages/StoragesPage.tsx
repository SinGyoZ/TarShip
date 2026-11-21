import { Button, Chip, Spinner } from "@heroui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorMessage } from "../api";
import { ConfirmModal } from "../components/ConfirmModal";
import { PageHeader } from "../components/PageHeader";
import { StorageFormModal } from "../components/StorageFormModal";
import type { Storage } from "../types";

export default function StoragesPage() {
  const qc = useQueryClient();
  const storages = useQuery({ queryKey: ["storages"], queryFn: api.listStorages });
  const servers = useQuery({ queryKey: ["servers"], queryFn: api.listServers });
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Storage | null>(null);
  const [toDelete, setToDelete] = useState<Storage | null>(null);
  // 行内展示最近一次测试结果（toast 依赖的 react-aria UNSTABLE API 在本环境不可靠）
  const [testResult, setTestResult] = useState<{ name: string; ok: boolean; message: string } | null>(null);

  const list = storages.data ?? [];

  // local 型的目录在被备份机器上，测试时挑一台在线服务器执行
  const testStorage = useMutation({
    mutationFn: async (st: Storage) => {
      if (st.type === "local") {
        const online = (servers.data ?? []).filter((s) => s.online);
        if (online.length === 0) throw new Error("没有在线的服务器可执行 local 目录测试");
        // 多台在线时取第一台；目录语义为 Agent 视角，换机器结果可能不同
        return api.testStorage(st.id, online[0].id);
      }
      return api.testStorage(st.id);
    },
    onSuccess: (r, st) => {
      setTestResult({ name: st.name, ok: r.ok, message: r.message });
    },
    onError: (e: Error) => {
      setTestResult({ name: "", ok: false, message: errorMessage(e) });
    },
  });

  return (
    <>
      <PageHeader
        title="存储器"
        description="备份产物的存放位置。S3 由 Agent 打包后直接上传；local 写被备份机器上的目录。"
        actions={<Button onPress={() => setFormOpen(true)}>添加存储器</Button>}
      />

      {testResult ? (
        <p role="status" className={`mb-3 rounded-lg border px-3 py-2 text-sm ${testResult.ok ? "border-success/40 text-success" : "border-danger/40 text-danger"}`}>
          {`${testResult.name ? `「${testResult.name}」` : ""}${testResult.message}`}
        </p>
      ) : null}

      {storages.isPending ? (
        <div className="flex items-center gap-3 py-10 text-muted" role="status">
          <Spinner size="sm" />
          <span>正在加载存储器…</span>
        </div>
      ) : storages.isError ? (
        <div role="alert">
          <p className="font-medium">加载失败</p>
          <p className="mt-1 text-sm text-muted">{errorMessage(storages.error)}</p>
        </div>
      ) : list.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-12 text-center">
          <p className="font-medium">还没有存储器</p>
          <p className="mx-auto mt-1 max-w-sm text-sm text-muted">
            备份产物需要一个存放位置：跨机备份选 S3 兼容存储，同机落盘选本地目录。
          </p>
          <Button className="mt-4" onPress={() => setFormOpen(true)}>
            添加存储器
          </Button>
        </div>
      ) : (
        <ul className="divide-y divide-border rounded-xl border border-border bg-surface">
          {list.map((st) => (
            <li key={st.id} className="grid gap-3 px-4 py-4 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto] md:items-center">
              <div className="min-w-0">
                <p className="truncate font-medium">{st.name}</p>
                <p className="mt-0.5 truncate font-mono text-xs text-muted">
                  {st.type === "local" ? st.root_dir : `${st.endpoint}/${st.bucket}`}
                </p>
              </div>
              <div className="flex flex-wrap gap-1">
                <Chip size="sm" variant="soft">
                  {st.type === "local" ? "本地目录" : "S3 兼容"}
                </Chip>
                {st.type === "s3" ? <Chip size="sm" variant="soft">{`前缀 ${st.prefix}`}</Chip> : null}
              </div>
              <div className="flex gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  isPending={testStorage.isPending && testStorage.variables?.id === st.id}
                  onPress={() => testStorage.mutate(st)}
                >
                  测试
                </Button>
                <Button size="sm" variant="ghost" onPress={() => setEditing(st)}>
                  编辑
                </Button>
                <Button size="sm" variant="danger-soft" onPress={() => setToDelete(st)}>
                  删除
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {formOpen || editing ? (
        <StorageFormModal
          editing={editing}
          onClose={() => {
            setFormOpen(false);
            setEditing(null);
          }}
          onSaved={() => {
            setFormOpen(false);
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ["storages"] });
          }}
        />
      ) : null}

      {toDelete ? (
        <ConfirmModal
          title={`删除存储器「${toDelete.name}」`}
          body="已被任务引用的存储器无法删除。删除后不影响已产生的备份产物。"
          confirmLabel="删除存储器"
          danger
          onClose={() => setToDelete(null)}
          onConfirm={async () => {
            try {
              await api.deleteStorage(toDelete.id);
              void qc.invalidateQueries({ queryKey: ["storages"] });
            } catch (e) {
              setTestResult({ name: "", ok: false, message: `删除失败：${errorMessage(e)}` });
            } finally {
              setToDelete(null);
            }
          }}
        />
      ) : null}
    </>
  );
}
