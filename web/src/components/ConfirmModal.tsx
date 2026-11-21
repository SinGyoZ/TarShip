import { Button, Modal, useOverlayState } from "@heroui/react";
import { useState } from "react";
import { errorMessage } from "../api";

interface Props {
  title: string;
  body: string;
  confirmLabel: string;
  danger?: boolean;
  onClose: () => void;
  onConfirm: () => Promise<void>;
}

/** 二次确认弹窗；执行失败时留在弹窗里显示原因，成功后由调用方关闭。 */
export function ConfirmModal({ title, body, confirmLabel, danger, onClose, onConfirm }: Props) {
  const state = useOverlayState({ defaultOpen: true, onOpenChange: (open) => !open && onClose() });
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    setPending(true);
    setError(null);
    try {
      await onConfirm();
      onClose();
    } catch (e) {
      setError(errorMessage(e));
      setPending(false);
    }
  };

  return (
    <Modal state={state}>
      <Modal.Backdrop>
        <Modal.Container>
          <Modal.Dialog>
            <Modal.Header>
              <Modal.Heading>{title}</Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <p className="text-sm">{body}</p>
              {error ? (
                <p role="alert" className="mt-3 text-sm text-danger">
                  {error}
                </p>
              ) : null}
            </Modal.Body>
            <Modal.Footer>
              <Button variant="secondary" onPress={onClose} isDisabled={pending}>
                取消
              </Button>
              <Button variant={danger ? "danger" : "primary"} onPress={run} isPending={pending}>
                {confirmLabel}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  );
}
