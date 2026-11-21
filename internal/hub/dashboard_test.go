package hub

import "testing"

func TestIsOutdated(t *testing.T) {
	cases := []struct {
		agent, hub string
		want       bool
	}{
		{"v0.1.1", "v0.1.1", false}, // 同版本
		{"v0.1.1", "v0.1.2", true},  // Agent 落后一个 patch
		{"v0.2.0", "v0.1.1", false}, // Agent 版本更新（不计落后）
		{"v0.1.1", "dev", false},    // Hub 为开发版，无法比较不计入
		{"dev", "v0.1.1", false},    // Agent 为开发版，无法比较不计入
		{"0.1.1", "v0.1.2", false},  // 无 v 前缀视为非 semver
	}
	for _, c := range cases {
		if got := isOutdated(c.agent, c.hub); got != c.want {
			t.Fatalf("isOutdated(%q, %q) = %v, 期望 %v", c.agent, c.hub, got, c.want)
		}
	}
}
