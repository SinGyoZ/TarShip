import { Button, Chip, Spinner } from "@heroui/react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorMessage } from "../api";
import { AddServerModal } from "../components/AddServerModal";
import { ConfirmModal } from "../components/ConfirmModal";
import { PageHeader } from "../components/PageHeader";
import { StatusDot } from "../components/StatusDot";
import { TokenModal } from "../components/TokenModal";
import { useNow } from "../hooks/useNow";
import { absoluteTime, relativeTime } from "../lib/format";
import type { Server, ServerWithToken } from "../types";

export default function ServersPage() {
  const qc = useQueryClient();
  const servers = useQuery({ queryKey: ["servers"], queryFn: api.listServers });
  const now = useNow(30_000);
  const [adding, setAdding] = useState(false);
  const [issued, setIssued] = useState<{ data: ServerWithToken; reset: boolean } | null>(null);
  const [toDelete, setToDelete] = useState<Server | null>(null);
  const [toReset, setToReset] = useState<Server | null>(null);

  const list = servers.data ?? [];
  const onlineCount = list.filter((s) => s.online).length;

  return (
    <>
      <PageHeader
        title="服务器"
        description={servers.data ? `共 ${list.length} 台，${onlineCount} 台在线` : undefined}
        actions={<Button onPress={() => setAdding(true)}>添加服务器</Button>}
      />

      {servers.isPending ? (
        <div className="flex items-center gap-3 py-10 text-muted" role="status">
          <Spinner size="sm" />
          <span>正在加载服务器列表…</span>
        </div>
      ) : servers.isError ? (
        <div role="alert" className="py-10">
          <p className="font-medium">加载服务器列表失败</p>
          <p className="mt-1 text-sm text-muted">{errorMessage(servers.error)}</p>
          <Button className="mt-4" variant="secondary" onPress={() => void servers.refetch()}>
            重试
          </Button>
        </div>
      ) : list.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-12 text-center">
          <p className="font-medium">还没有服务器</p>
          <p className="mx-auto mt-1 max-w-sm text-sm text-muted">添加第一台服务器，拿到 token 后在目标机器上运行 Agent，它就会出现在这里。</p>
          <Button className="mt-4" onPress={() => setAdding(true)}>
            添加服务器
          </Button>
        </div>
      ) : (
        <ul className="divide-y divide-border rounded-xl border border-border bg-surface">
          {list.map((s) => (
            <li key={s.id} className="grid gap-3 px-4 py-4 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto] md:items-center">
              <div className="min-w-0">
                <p className="truncate font-medium">{s.name}</p>
                <p className="mt-0.5 truncate font-mono text-xs text-muted">{s.hostname || "尚未连接"}</p>
                {s.tags.length > 0 ? (
                  <div className="mt-1.5 flex flex-wrap gap-1">
                    {s.tags.map((t) => (
                      <Chip key={t} size="sm" variant="soft">
                        {t}
                      </Chip>
                    ))}
                  </div>
                ) : null}
              </div>
              <div className="min-w-0 space-y-0.5">
                <StatusDot online={s.online} />
                <p className="truncate text-xs text-muted">
                  {s.online ? (
                    s.agent_version ? `Agent ${s.agent_version}` : null
                  ) : s.last_seen_at ? (
                    <time dateTime={s.last_seen_at} title={absoluteTime(s.last_seen_at)}>
                      最后在线 {relativeTime(s.last_seen_at, now)}
                    </time>
                  ) : (
                    "从未连接"
                  )}
                </p>
              </div>
              <div className="flex gap-1">
                <Button size="sm" variant="ghost" onPress={() => setToReset(s)}>
                  重置 token
                </Button>
                <Button size="sm" variant="danger-soft" onPress={() => setToDelete(s)}>
                  删除
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {adding ? (
        <AddServerModal
          onClose={() => setAdding(false)}
          onCreated={(data) => {
            setAdding(false);
            setIssued({ data, reset: false });
          }}
        />
      ) : null}

      {issued ? <TokenModal issued={issued.data} reset={issued.reset} onClose={() => setIssued(null)} /> : null}

      {toReset ? (
        <ConfirmModal
          title={`重置 ${toReset.name} 的 token`}
          body="旧 token 会立即失效，使用它的 Agent 会被断开。你需要把新 token 配置到该服务器上，Agent 才能重新上线。"
          confirmLabel="重置 token"
          danger
          onClose={() => setToReset(null)}
          onConfirm={async () => {
            const data = await api.resetToken(toReset.id);
            await qc.invalidateQueries({ queryKey: ["servers"] });
            setIssued({ data, reset: true });
          }}
        />
      ) : null}

      {toDelete ? (
        <ConfirmModal
          title={`删除 ${toDelete.name}`}
          body="删除后该服务器的 token 立即失效，在线的 Agent 会被断开。此操作无法撤销。"
          confirmLabel="删除服务器"
          danger
          onClose={() => setToDelete(null)}
          onConfirm={async () => {
            await api.deleteServer(toDelete.id);
            await qc.invalidateQueries({ queryKey: ["servers"] });
          }}
        />
      ) : null}
    </>
  );
}
