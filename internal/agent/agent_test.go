package agent

import (
	"testing"
	"time"
)

func TestWSURL(t *testing.T) {
	good := []struct {
		in       string
		insecure bool
		want     string
	}{
		{"wss://hub.example.com", false, "wss://hub.example.com/ws/agent"},
		{"wss://hub.example.com/", false, "wss://hub.example.com/ws/agent"},
		{"wss://hub.example.com:8443/prefix", false, "wss://hub.example.com:8443/prefix/ws/agent"},
		{"ws://127.0.0.1:8080", true, "ws://127.0.0.1:8080/ws/agent"},
	}
	for _, c := range good {
		got, err := WSURL(c.in, c.insecure)
		if err != nil || got != c.want {
			t.Errorf("WSURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	bad := []struct {
		in       string
		insecure bool
	}{
		{"ws://hub.example.com", false},
		{"https://hub.example.com", true},
		{"hub.example.com", false},
		{"", false},
	}
	for _, c := range bad {
		if got, err := WSURL(c.in, c.insecure); err == nil {
			t.Errorf("WSURL(%q) 应报错, got %q", c.in, got)
		}
	}
}

func TestBackoff(t *testing.T) {
	b := newBackoff(time.Second, 8*time.Second)
	base := []time.Duration{1, 2, 4, 8, 8, 8}
	for i, want := range base {
		d := b.next()
		lo, hi := time.Duration(float64(want*time.Second)*0.8), time.Duration(float64(want*time.Second)*1.2)
		if d < lo || d > hi {
			t.Fatalf("第 %d 次退避 %v 不在 [%v, %v]", i, d, lo, hi)
		}
	}
	b.reset()
	if d := b.next(); d > 1200*time.Millisecond {
		t.Fatalf("reset 后应从起点开始, got %v", d)
	}
}
