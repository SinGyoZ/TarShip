import { Button, FieldError, Input, Label, Modal, TextField, useOverlayState } from "@heroui/react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, errorMessage } from "../api";
import type { ServerWithToken } from "../types";

export function AddServerModal({ onClose, onCreated }: { onClose: () => void; onCreated: (r: ServerWithToken) => void }) {
  const state = useOverlayState({ defaultOpen: true, onOpenChange: (open) => !open && onClose() });
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: api.createServer,
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: ["servers"] });
      onCreated(r);
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (name.trim()) create.mutate(name.trim());
  };

  return (
    <Modal state={state}>
      <Modal.Backdrop>
        <Modal.Container>
          <Modal.Dialog>
            <form onSubmit={submit}>
              <Modal.Header>
                <Modal.Heading>添加服务器</Modal.Heading>
              </Modal.Header>
              <Modal.Body>
                <TextField value={name} onChange={setName} isRequired autoFocus maxLength={64} name="name">
                  <Label>服务器名称</Label>
                  <Input placeholder="例如：web-01" />
                  <FieldError />
                </TextField>
                <p className="mt-2 text-sm text-muted">创建后会生成一个只显示一次的 Agent token，用它把目标服务器连接到 Hub。</p>
                {create.error ? (
                  <p role="alert" className="mt-3 text-sm text-danger">
                    {errorMessage(create.error)}
                  </p>
                ) : null}
              </Modal.Body>
              <Modal.Footer>
                <Button type="button" variant="secondary" onPress={onClose} isDisabled={create.isPending}>
                  取消
                </Button>
                <Button type="submit" isPending={create.isPending}>
                  创建
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}
