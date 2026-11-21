import { Button, FieldError, Input, Label, Modal, TextField, useOverlayState } from "@heroui/react";
import { useState, type FormEvent } from "react";
import { api, errorMessage } from "../api";
import type { Storage, StorageInput } from "../types";

const field =
  "w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent";

/** 创建/编辑存储器。type 切换决定表单字段。 */
export function StorageFormModal({
  editing,
  onClose,
  onSaved,
}: {
  editing: Storage | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const state = useOverlayState({ defaultOpen: true, onOpenChange: (open) => !open && onClose() });
  const [type, setType] = useState<"local" | "s3">(editing?.type ?? "local");
  const [name, setName] = useState(editing?.name ?? "");
  const [rootDir, setRootDir] = useState(editing?.root_dir ?? "");
  const [endpoint, setEndpoint] = useState(editing?.endpoint ?? "");
  const [region, setRegion] = useState(editing?.region ?? "");
  const [bucket, setBucket] = useState(editing?.bucket ?? "");
  const [accessKeyID, setAccessKeyID] = useState(editing?.access_key_id ?? "");
  const [secretKey, setSecretKey] = useState("");
  const [prefix, setPrefix] = useState(editing?.prefix ?? "");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    const body: StorageInput = { name, type };
    if (type === "local") {
      body.root_dir = rootDir;
    } else {
      body.endpoint = endpoint;
      body.region = region;
      body.bucket = bucket;
      body.access_key_id = accessKeyID;
      body.secret_access_key = secretKey; // 编辑时留空 = 沿用旧值
      body.prefix = prefix;
    }
    setPending(true);
    try {
      if (editing) {
        await api.updateStorage(editing.id, body);
      } else {
        await api.createStorage(body);
      }
      onSaved();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <Modal state={state}>
      <Modal.Backdrop>
        <Modal.Container>
          <Modal.Dialog>
            <form onSubmit={submit}>
              <Modal.Header>
                <Modal.Heading>{editing ? "编辑存储器" : "添加存储器"}</Modal.Heading>
              </Modal.Header>
              <Modal.Body className="space-y-4">
                <div>
                  <p className="mb-2 text-sm text-muted">类型</p>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      variant={type === "local" ? "primary" : "secondary"}
                      onPress={() => setType("local")}
                    >
                      本地目录
                    </Button>
                    <Button
                      type="button"
                      variant={type === "s3" ? "primary" : "secondary"}
                      onPress={() => setType("s3")}
                    >
                      S3 兼容
                    </Button>
                  </div>
                  <p className="mt-2 text-xs text-muted">
                    {type === "local"
                      ? "写入被备份机器上的目录（存储器与该机器同机时使用），路径为 Agent 视角。注意：容器化 Agent 使用 local 存储时，必须把存储目录以同一路径挂载进容器，否则产物会写进容器内部，容器删除即丢失。"
                      : "S3 / MinIO / R2 / B2 等兼容端点，Agent 打包后直接上传，数据不流经 Hub。跨机备份推荐此方式。"}
                  </p>
                </div>
                <TextField value={name} onChange={setName} isRequired autoFocus maxLength={64} name="name">
                  <Label>名称</Label>
                  <Input placeholder="例如：本机备份" />
                  <FieldError />
                </TextField>
                {type === "local" ? (
                  <div>
                    <label htmlFor="storage-root" className="mb-1 block text-sm">
                      存储目录（Agent 视角绝对路径）
                    </label>
                    <input
                      id="storage-root"
                      className={field}
                      value={rootDir}
                      onChange={(e) => setRootDir(e.target.value)}
                      placeholder="/var/tarship-backups"
                      required
                    />
                  </div>
                ) : (
                  <>
                    <div>
                      <label htmlFor="storage-endpoint" className="mb-1 block text-sm">
                        Endpoint
                      </label>
                      <input
                        id="storage-endpoint"
                        className={field}
                        value={endpoint}
                        onChange={(e) => setEndpoint(e.target.value)}
                        placeholder="s3.example.com 或 https://s3.example.com（默认 https）"
                        required
                      />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label htmlFor="storage-bucket" className="mb-1 block text-sm">
                          Bucket
                        </label>
                        <input
                          id="storage-bucket"
                          className={field}
                          value={bucket}
                          onChange={(e) => setBucket(e.target.value)}
                          required
                        />
                      </div>
                      <div>
                        <label htmlFor="storage-region" className="mb-1 block text-sm">
                          Region（可留空）
                        </label>
                        <input
                          id="storage-region"
                          className={field}
                          value={region}
                          onChange={(e) => setRegion(e.target.value)}
                        />
                      </div>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label htmlFor="storage-ak" className="mb-1 block text-sm">
                          Access Key ID
                        </label>
                        <input
                          id="storage-ak"
                          className={field}
                          value={accessKeyID}
                          onChange={(e) => setAccessKeyID(e.target.value)}
                          required
                        />
                      </div>
                      <div>
                        <label htmlFor="storage-sk" className="mb-1 block text-sm">
                          Secret Access Key{editing ? "（留空沿用原值）" : ""}
                        </label>
                        <input
                          id="storage-sk"
                          type="password"
                          className={field}
                          value={secretKey}
                          onChange={(e) => setSecretKey(e.target.value)}
                          required={!editing}
                        />
                      </div>
                    </div>
                    <div>
                      <label htmlFor="storage-prefix" className="mb-1 block text-sm">
                        对象键前缀（默认 backups）
                      </label>
                      <input
                        id="storage-prefix"
                        className={field}
                        value={prefix}
                        onChange={(e) => setPrefix(e.target.value)}
                        placeholder="backups"
                      />
                    </div>
                  </>
                )}
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
                <Button type="submit" isPending={pending}>
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
