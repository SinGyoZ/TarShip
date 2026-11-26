import { Button, Modal, useOverlayState } from "@heroui/react";
import { useState } from "react";
import { agentInstallSnippet, agentUninstallSnippet, insecureNote } from "../lib/install";
import type { ServerWithToken } from "../types";
import { CodeBlock } from "./CodeBlock";

/** 展示新建或重置后的 token。token 明文只在这里出现一次，所以不允许点击遮罩误关。 */
export function TokenModal({ issued, reset, onClose }: { issued: ServerWithToken; reset: boolean; onClose: () => void }) {
  const state = useOverlayState({ defaultOpen: true, onOpenChange: (open) => !open && onClose() });
  const [arch, setArch] = useState<"amd64" | "arm64">("amd64");
  const snippet = agentInstallSnippet(issued.token, location, arch);
  const note = insecureNote(location);
  return (
    <Modal state={state}>
      <Modal.Backdrop isDismissable={false} isKeyboardDismissDisabled>
        <Modal.Container size="lg">
          <Modal.Dialog>
            <Modal.Header>
              <Modal.Heading>{reset ? `${issued.server.name} 的新 token` : `已添加 ${issued.server.name}`}</Modal.Heading>
            </Modal.Header>
            <Modal.Body className="space-y-4">
              <p role="alert" className="text-sm">
                token 只会显示这一次，关闭后无法再次查看；丢失时可以在服务器列表里重置。
                {reset ? " 旧 token 已失效，使用旧 token 的 Agent 已被断开。" : ""}
              </p>
              <CodeBlock label="Agent token" value={issued.token} />
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted">目标服务器架构</span>
                <Button variant={arch === "amd64" ? "primary" : "secondary"} onPress={() => setArch("amd64")}>
                  amd64
                </Button>
                <Button variant={arch === "arm64" ? "primary" : "secondary"} onPress={() => setArch("arm64")}>
                  arm64
                </Button>
              </div>
              <CodeBlock
                label="在目标服务器上以 root 执行（下载→校验→安装→启动，一键完成；升级重装同命令）"
                value={snippet}
              />
              {note ? <p className="text-xs text-warning">{note}</p> : null}
              <CodeBlock label="卸载命令（停止服务并删除二进制与配置，不影响已有备份）" value={agentUninstallSnippet()} />
            </Modal.Body>
            <Modal.Footer>
              <Button onPress={onClose}>我已保存，关闭</Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}
