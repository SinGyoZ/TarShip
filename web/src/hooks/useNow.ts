import { useEffect, useState } from "react";

/** 返回当前时间戳并按间隔刷新，用于让「3 分钟前」这类相对时间保持准确。 */
export function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}
