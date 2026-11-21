package hub

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("第 %d 次失败前应放行", i)
		}
		l.fail("a")
	}
	ok, retry := l.allow("a")
	if ok || retry <= 0 || retry > time.Minute {
		t.Fatalf("达到上限后应拒绝: ok=%v retry=%v", ok, retry)
	}
	if ok, _ := l.allow("b"); !ok {
		t.Fatal("其他 key 不应受影响")
	}

	now = now.Add(time.Minute + time.Second)
	if ok, _ := l.allow("a"); !ok {
		t.Fatal("窗口过后应恢复放行")
	}
}

func TestLimiterReset(t *testing.T) {
	l := newLimiter(1, time.Minute)
	l.fail("a")
	if ok, _ := l.allow("a"); ok {
		t.Fatal("应被拒绝")
	}
	l.reset("a")
	if ok, _ := l.allow("a"); !ok {
		t.Fatal("reset 后应放行")
	}
}
