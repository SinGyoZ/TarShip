const rtf = new Intl.RelativeTimeFormat("zh-CN", { numeric: "auto" });

/** 把 ISO 时间格式化成「3 分钟前」这样的相对时间。 */
export function relativeTime(iso: string, now: number = Date.now()): string {
  const seconds = Math.round((new Date(iso).getTime() - now) / 1000);
  const abs = Math.abs(seconds);
  if (abs < 45) return "刚刚";
  if (abs < 3600) return rtf.format(Math.round(seconds / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(seconds / 3600), "hour");
  return rtf.format(Math.round(seconds / 86400), "day");
}

export function absoluteTime(iso: string): string {
  return new Date(iso).toLocaleString("zh-CN", { hour12: false });
}
