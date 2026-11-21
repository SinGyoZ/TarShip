package hub

import (
	"sync"
	"time"
)

// limiter 按 key（客户端 IP）限制失败次数：窗口内失败达到上限后，直到窗口结束前一律拒绝。
// 状态只在内存里，重启即清零，对单实例 Hub 足够。
type limiter struct {
	mu      sync.Mutex
	entries map[string]*limitEntry
	max     int
	window  time.Duration
	now     func() time.Time
}

type limitEntry struct {
	fails int
	start time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{entries: map[string]*limitEntry{}, max: max, window: window, now: time.Now}
}

// allow 返回是否放行；被拒绝时 retry 是还需等待的时间。
func (l *limiter) allow(key string) (ok bool, retry time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[key]
	if e == nil {
		return true, 0
	}
	now := l.now()
	if now.Sub(e.start) >= l.window {
		delete(l.entries, key)
		return true, 0
	}
	if e.fails >= l.max {
		return false, e.start.Add(l.window).Sub(now)
	}
	return true, 0
}

// fail 记录一次失败。
func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.entries) > 10000 { // 防止恶意构造大量 key 撑爆内存
		for k, e := range l.entries {
			if now.Sub(e.start) >= l.window {
				delete(l.entries, k)
			}
		}
	}
	e := l.entries[key]
	if e == nil || now.Sub(e.start) >= l.window {
		e = &limitEntry{start: now}
		l.entries[key] = e
	}
	e.fails++
}

// reset 清除某个 key 的失败记录（登录成功时调用）。
func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
