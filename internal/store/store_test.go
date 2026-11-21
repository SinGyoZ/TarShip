package store

import (
	"context"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		s, err := Open(dir)
		if err != nil {
			t.Fatalf("第 %d 次打开失败: %v", i+1, err)
		}
		s.Close()
	}
}

func TestServerLifecycle(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	sv, err := s.CreateServer(ctx, "vps-01", []string{"web", "prod"}, "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if sv.ID == 0 {
		t.Fatal("应返回自增 ID")
	}
	if _, err := s.CreateServer(ctx, "dup", nil, "hash-1"); err == nil {
		t.Fatal("token_hash 重复应报错")
	}

	got, ok, err := s.GetServerByTokenHash(ctx, "hash-1")
	if err != nil || !ok || got.Name != "vps-01" || len(got.Tags) != 2 {
		t.Fatalf("按 token 哈希查询不对: %+v ok=%v err=%v", got, ok, err)
	}
	if got.LastSeenAt != nil {
		t.Fatal("新建的服务器不应有 last_seen_at")
	}

	seen := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.TouchServer(ctx, sv.ID, "host-a", "0.1.0", seen); err != nil {
		t.Fatal(err)
	}
	got, _, _ = s.GetServer(ctx, sv.ID)
	if got.Hostname != "host-a" || got.AgentVersion != "0.1.0" || got.LastSeenAt == nil || !got.LastSeenAt.Equal(seen) {
		t.Fatalf("TouchServer 结果不对: %+v", got)
	}

	if ok, err := s.UpdateServerToken(ctx, sv.ID, "hash-2"); err != nil || !ok {
		t.Fatalf("重置 token: ok=%v err=%v", ok, err)
	}
	if _, ok, _ := s.GetServerByTokenHash(ctx, "hash-1"); ok {
		t.Fatal("旧 token 应失效")
	}
	if _, ok, _ := s.GetServerByTokenHash(ctx, "hash-2"); !ok {
		t.Fatal("新 token 应生效")
	}

	list, err := s.ListServers(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListServers = %v, %v", list, err)
	}

	if ok, err := s.DeleteServer(ctx, sv.ID); err != nil || !ok {
		t.Fatalf("删除: ok=%v err=%v", ok, err)
	}
	if ok, _ := s.DeleteServer(ctx, sv.ID); ok {
		t.Fatal("重复删除应返回 false")
	}
	list, _ = s.ListServers(ctx)
	if len(list) != 0 {
		t.Fatal("删除后列表应为空")
	}
}

func TestCreateAdminOnlyOnce(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	if _, _, ok, err := s.Admin(ctx); err != nil || ok {
		t.Fatalf("初始不应有管理员: ok=%v err=%v", ok, err)
	}
	if created, err := s.CreateAdmin(ctx, "root", "hash"); err != nil || !created {
		t.Fatalf("首次创建: created=%v err=%v", created, err)
	}
	if created, err := s.CreateAdmin(ctx, "other", "hash2"); err != nil || created {
		t.Fatalf("重复创建应返回 false: created=%v err=%v", created, err)
	}
	user, hash, ok, err := s.Admin(ctx)
	if err != nil || !ok || user != "root" || hash != "hash" {
		t.Fatalf("Admin = %q %q %v %v", user, hash, ok, err)
	}
}

func TestEnsureSettingStable(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	n := 0
	gen := func() (string, error) { n++; return "v" + string(rune('0'+n)), nil }

	a, err := s.EnsureSetting(ctx, "k", gen)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.EnsureSetting(ctx, "k", gen)
	if a != b || n != 1 {
		t.Fatalf("已存在时不应再次生成: a=%q b=%q n=%d", a, b, n)
	}
}
