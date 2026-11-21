import { Button } from "@heroui/react";
import { useState } from "react";

function CopyButton({ text }: { text: string }) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  const onPress = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setStatus("copied");
    } catch {
      // 非 HTTPS 页面里浏览器会禁用剪贴板接口，提示手动选择即可。
      setStatus("failed");
    }
    window.setTimeout(() => setStatus("idle"), 2500);
  };
  return (
    <Button size="sm" variant="ghost" onPress={onPress}>
      {status === "copied" ? "已复制" : status === "failed" ? "复制失败，请手动选择" : "复制"}
    </Button>
  );
}

export function CodeBlock({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{label}</span>
        <CopyButton text={value} />
      </div>
      {/* tabIndex 让键盘用户也能聚焦并滚动长内容 */}
      <pre
        tabIndex={0}
        className="mt-1 max-h-64 overflow-auto rounded-lg bg-surface-secondary p-3 font-mono text-xs leading-relaxed"
      >
        {value}
      </pre>
    </div>
  );
}
