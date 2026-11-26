package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenUnwritableDirFriendlyError 验证权限场景下报错带可执行修复指引，
// 而不是裸的 SQLite 错误码（用户实际踩过这个坑）。
func TestOpenUnwritableDirFriendlyError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 环境下权限探测恒通过，跳过")
	}
	dir := t.TempDir()
	// 去掉全部写权限
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := Open(dir)
	if err == nil {
		t.Fatal("只读目录应报错")
	}
	msg := err.Error()
	for _, want := range []string{"不可写", "chown -R 65532:65532", "user: \"65532:65532\"", "TARSHIP_DATA"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误信息缺少关键指引 %q:\n%s", want, msg)
		}
	}
	// 探针临时文件不应残留
	matches, _ := filepath.Glob(filepath.Join(dir, ".write-probe-*"))
	if len(matches) != 0 {
		t.Fatalf("探针残留: %v", matches)
	}
}
