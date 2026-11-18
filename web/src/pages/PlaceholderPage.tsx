import { Card, CardBody } from '@heroui/react';

// M0 未交付页面的占位：明确标注功能所属里程碑，避免用户误以为功能缺失是缺陷
export default function PlaceholderPage({
  title,
  milestone,
  description,
}: {
  title: string;
  milestone: string;
  description: string;
}) {
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-xl font-bold">{title}</h1>
        <p className="mt-1 text-sm text-foreground-500">本模块将在 {milestone} 里程碑交付</p>
      </div>
      <Card shadow="sm">
        <CardBody className="flex flex-col items-center gap-3 py-16 text-center">
          <span aria-hidden className="text-4xl opacity-40">
            🚧
          </span>
          <p className="max-w-md text-sm text-foreground-500">{description}</p>
          <p className="text-xs text-foreground-400">
            当前进度为 M0（骨架）：服务器接入与在线状态已可用
          </p>
        </CardBody>
      </Card>
    </div>
  );
}
