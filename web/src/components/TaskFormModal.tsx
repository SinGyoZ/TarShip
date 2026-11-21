import { Button, FieldError, Input, Label, Modal, TextField, useOverlayState } from "@heroui/react";
import { useEffect, useState, type FormEvent } from "react";
import { api, errorMessage } from "../api";
import type { Server, Storage, Task, TaskInput } from "../types";

const field =
  "w-full rounded-lg border border-border bg-surface px-3 py-2 font-mono text-sm outline-none focus:border-accent";

/** 多行文本拆成路径/规则数组：忽略空行与首尾空白。 */
const splitLines = (v: string): string[] =>
  v
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l !== "");

/** 创建/编辑备份任务。 */
export function TaskFormModal({
  editing,
  servers,
  storages,
  onClose,
  onSaved,
}: {
  editing: Task | null;
  servers: Server[];
  storages: Storage[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const state = useOverlayState({ defaultOpen: true, onOpenChange: (open) => !open && onClose() });
  const [name, setName] = useState(editing?.name ?? "");
  const [serverID, setServerID] = useState<number>(editing?.server_id ?? servers[0]?.id ?? 0);
  const [storageID, setStorageID] = useState<number>(editing?.storage_id ?? storages[0]?.id ?? 0);
  const [paths, setPaths] = useState((editing?.paths ?? []).join("\n"));
  const [excludes, setExcludes] = useState((editing?.excludes ?? []).join("\n"));
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  // 列表后到时补默认选中项（弹窗打开早于数据到达的情况）
  useEffect(() => {
    if (!editing) {
      if (!serverID && servers.length > 0) setServerID(servers[0].id);
      if (!storageID && storages.length > 0) setStorageID(storages[0].id);
    }
  }, [servers, storages, editing, serverID, storageID]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    const body: TaskInput = {
      name,
      server_id: serverID,
      storage_id: storageID,
      paths: splitLines(paths),
      excludes: splitLines(excludes),
    };
    if (body.paths.length === 0) {
      setError("至少配置一个要备份的目录");
      return;
    }
    setPending(true);
    try {
      if (editing) {
        await api.updateTask(editing.id, body);
      } else {
        await api.createTask(body);
      }
      onSaved();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  const disabled = servers.length === 0 || storages.length === 0;

  return (
    <Modal state={state}>
      <Modal.Backdrop>
        <Modal.Container size="lg">
          <Modal.Dialog>
            <form onSubmit={submit}>
              <Modal.Header>
                <Modal.Heading>{editing ? "编辑任务" : "新建备份任务"}</Modal.Heading>
              </Modal.Header>
              <Modal.Body className="space-y-4">
                {disabled ? (
                  <p role="alert" className="text-sm text-warning">
                    {servers.length === 0 ? "还没有服务器，请先到「服务器」页接入。" : ""}
                    {storages.length === 0 ? " 还没有存储器，请先到「存储器」页添加。" : ""}
                  </p>
                ) : null}
                <TextField value={name} onChange={setName} isRequired autoFocus maxLength={64} name="name">
                  <Label>任务名称</Label>
                  <Input placeholder="例如：网站数据每日备份" />
                  <FieldError />
                </TextField>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label htmlFor="task-server" className="mb-1 block text-sm">
                      目标服务器
                    </label>
                    <select
                      id="task-server"
                      className={field}
                      value={serverID}
                      onChange={(e) => setServerID(Number(e.target.value))}
                      required
                    >
                      {servers.map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.name}
                          {s.online ? "" : "（离线）"}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label htmlFor="task-storage" className="mb-1 block text-sm">
                      存储器
                    </label>
                    <select
                      id="task-storage"
                      className={field}
                      value={storageID}
                      onChange={(e) => setStorageID(Number(e.target.value))}
                      required
                    >
                      {storages.map((st) => (
                        <option key={st.id} value={st.id}>
                          {st.name}（{st.type === "local" ? "本地目录" : "S3"}）
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
                <div>
                  <label htmlFor="task-paths" className="mb-1 block text-sm">
                    备份目录（每行一个，Agent 视角绝对路径）
                  </label>
                  <textarea
                    id="task-paths"
                    className={`${field} min-h-20`}
                    value={paths}
                    onChange={(e) => setPaths(e.target.value)}
                    placeholder={"/var/www\n/etc/nginx"}
                    required
                  />
                </div>
                <div>
                  <label htmlFor="task-excludes" className="mb-1 block text-sm">
                    排除规则（每行一个，支持 ** 递归 glob，可选）
                  </label>
                  <textarea
                    id="task-excludes"
                    className={`${field} min-h-16`}
                    value={excludes}
                    onChange={(e) => setExcludes(e.target.value)}
                    placeholder={"**/node_modules/**\n**/*.log"}
                  />
                </div>
                {error ? (
                  <p role="alert" className="text-sm text-danger">
                    {error}
                  </p>
                ) : null}
              </Modal.Body>
              <Modal.Footer>
                <Button type="button" variant="secondary" onPress={onClose}>
                  取消
                </Button>
                <Button type="submit" isPending={pending} isDisabled={disabled}>
                  {editing ? "保存" : "创建"}
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}
