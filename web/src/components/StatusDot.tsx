import { cn } from "@heroui/react";

/** 在线状态：圆点 + 文字，不只靠颜色传递信息；动效仅在在线时出现，且尊重「减少动态效果」设置。 */
export function StatusDot({ online }: { online: boolean }) {
  return (
    <span className="inline-flex items-center gap-2 text-sm">
      <span className="relative flex size-2.5" aria-hidden="true">
        {online ? (
          <span className="absolute inline-flex size-full rounded-full bg-success opacity-60 motion-safe:animate-ping" />
        ) : null}
        <span className={cn("relative inline-flex size-2.5 rounded-full", online ? "bg-success" : "bg-muted/50")} />
      </span>
      <span className={online ? "font-medium" : "text-muted"}>{online ? "在线" : "离线"}</span>
    </span>
  );
}
