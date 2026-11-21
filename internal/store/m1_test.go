package store

// M1 四表的数据访问与状态机测试。

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestStorageCRUDAndValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	st0 := Storage{Name: "本机备份", Type: "local", RootDir: "/var/backups"}
	if err := ValidateStorage(&st0); err != nil {
		t.Fatalf("local 校验: %v", err)
	}
	if st0.Prefix != "backups" {
		t.Fatalf("local 前缀默认值: %q", st0.Prefix)
	}
	created, err := st.CreateStorage(ctx, st0)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Prefix != "backups" {
		t.Fatalf("创建结果异常: %+v", created)
	}

	// s3 完整字段 + 默认前缀
	s3 := Storage{Name: "r2", Type: "s3", Endpoint: "abc.r2.cloudflarestorage.com", Bucket: "tarship", AccessKeyID: "ak", SecretAccessKey: "sk"}
	if err := ValidateStorage(&s3); err != nil {
		t.Fatal(err)
	}
	s3Created, err := st.CreateStorage(ctx, s3)
	if err != nil {
		t.Fatal(err)
	}
	if s3Created.Prefix != "backups" {
		t.Fatalf("s3 默认前缀: %q", s3Created.Prefix)
	}

	// 非法输入
	bad := Storage{Name: "x", Type: "ftp"}
	if err := ValidateStorage(&bad); err == nil {
		t.Fatal("未知类型应被拒绝")
	}
	badLocal := Storage{Name: "x", Type: "local", RootDir: "relative/path"}
	if err := ValidateStorage(&badLocal); err == nil {
		t.Fatal("相对路径 local 应被拒绝")
	}
	badS3 := Storage{Name: "x", Type: "s3", Endpoint: "e", Bucket: "b"}
	if err := ValidateStorage(&badS3); err == nil {
		t.Fatal("缺凭据 s3 应被拒绝")
	}

	// 更新
	s3Created.Name = "r2-renamed"
	updated, err := st.UpdateStorage(ctx, s3Created.ID, s3Created)
	if err != nil || updated.Name != "r2-renamed" {
		t.Fatalf("更新失败: %v %+v", err, updated)
	}

	list, err := st.ListStorages(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("列表应含 2 个存储器: %v %v", err, len(list))
	}

	// 删除：先建任务引用再删应被拒
	srv, _, _, err := st.Admin(ctx) // 占位避免未使用
	_ = srv
	_ = err
	server := seedServer(t, st)
	if _, err := st.CreateTask(ctx, Task{Name: "t", ServerID: server, StorageID: created.ID, Paths: []string{"/data"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteStorage(ctx, created.ID); err == nil {
		t.Fatal("被任务引用的存储器不应可删除")
	}
	ok, err := st.DeleteStorage(ctx, s3Created.ID)
	if err != nil || !ok {
		t.Fatalf("删除未引用存储器失败: %v %v", ok, err)
	}
	_, err = st.GetStorage(ctx, s3Created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后应 not found: %v", err)
	}
}

func seedServer(t *testing.T, st *Store) int64 {
	t.Helper()
	sv, err := st.CreateServer(context.Background(), "srv", nil, fmt.Sprintf("hash-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	return sv.ID
}

func TestTaskValidationAndJSONRoundtrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	server := seedServer(t, st)
	storage, err := st.CreateStorage(ctx, Storage{Name: "s", Type: "local", RootDir: "/b"})
	if err != nil {
		t.Fatal(err)
	}

	tk := Task{Name: "网站数据", ServerID: server, StorageID: storage.ID,
		Paths: []string{"/var/www"}, Excludes: []string{"**/node_modules/**"}, Enabled: true}
	if err := ValidateTask(&tk); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateTask(ctx, tk)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Paths) != 1 || got.Paths[0] != "/var/www" || len(got.Excludes) != 1 {
		t.Fatalf("JSON 往返异常: %+v", got)
	}
	if !got.Enabled {
		t.Fatal("默认应启用")
	}

	// 非法任务
	bad := Task{Name: "x", ServerID: 0, StorageID: storage.ID, Paths: []string{"/a"}}
	if err := ValidateTask(&bad); err == nil {
		t.Fatal("缺 server 应拒绝")
	}
	bad = Task{Name: "x", ServerID: server, StorageID: storage.ID, Paths: []string{"var/rel"}}
	if err := ValidateTask(&bad); err == nil {
		t.Fatal("相对路径应拒绝")
	}
	bad = Task{Name: "x", ServerID: server, StorageID: storage.ID, Paths: nil}
	if err := ValidateTask(&bad); err == nil {
		t.Fatal("空 paths 应拒绝")
	}
}

func TestRunLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	server := seedServer(t, st)
	storage, _ := st.CreateStorage(ctx, Storage{Name: "s", Type: "local", RootDir: "/b"})
	tk, err := st.CreateTask(ctx, Task{Name: "t", ServerID: server, StorageID: storage.ID, Paths: []string{"/a"}})
	if err != nil {
		t.Fatal(err)
	}

	// 并发防护：running 存在时拒绝创建
	r1, err := st.CreateRun(ctx, tk.ID, server, storage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Status != "running" || r1.StartedAt.IsZero() {
		t.Fatalf("初始状态异常: %+v", r1)
	}
	if _, err := st.CreateRun(ctx, tk.ID, server, storage.ID); err == nil {
		t.Fatal("running 期间应拒绝重复创建")
	}

	// 成功结算：落状态 + 登记产物，重复结算被拒
	if err := st.FinishRunSuccess(ctx, r1.ID, "backups/1/1/x.tar.zst", 123, "abc"); err != nil {
		t.Fatal(err)
	}
	finished, _ := st.GetRun(ctx, r1.ID)
	if finished.Status != "success" || finished.FinishedAt == nil || finished.SizeBytes != 123 {
		t.Fatalf("成功结算异常: %+v", finished)
	}
	backups, err := st.ListBackups(ctx, 10)
	if err != nil || len(backups) != 1 {
		t.Fatalf("产物登记缺失: %v %d", err, len(backups))
	}
	if backups[0].Sha256 != "abc" || backups[0].SizeBytes != 123 {
		t.Fatalf("产物字段异常: %+v", backups[0])
	}
	if err := st.FinishRunSuccess(ctx, r1.ID, "again", 1, "x"); err == nil {
		t.Fatal("重复结算应被拒绝")
	}

	// 失败路径
	r2, _ := st.CreateRun(ctx, tk.ID, server, storage.ID)
	if err := st.FinishRunFailed(ctx, r2.ID, "路径不可访问"); err != nil {
		t.Fatal(err)
	}
	f2, _ := st.GetRun(ctx, r2.ID)
	if f2.Status != "failed" || f2.ErrorMsg != "路径不可访问" || f2.FinishedAt == nil {
		t.Fatalf("失败结算异常: %+v", f2)
	}
	backups2, _ := st.ListBackups(ctx, 10)
	if len(backups2) != 1 {
		t.Fatal("失败运行不应登记产物")
	}
}

func TestFailServerRunsAndStaleSweep(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	server := seedServer(t, st)
	other := seedServer(t, st)
	storage, _ := st.CreateStorage(ctx, Storage{Name: "s", Type: "local", RootDir: "/b"})
	tk1, _ := st.CreateTask(ctx, Task{Name: "t1", ServerID: server, StorageID: storage.ID, Paths: []string{"/a"}})
	tk2, _ := st.CreateTask(ctx, Task{Name: "t2", ServerID: other, StorageID: storage.ID, Paths: []string{"/b"}})

	r1, _ := st.CreateRun(ctx, tk1.ID, server, storage.ID)
	_, _ = st.CreateRun(ctx, tk2.ID, other, storage.ID)

	// Agent 断线：只清该服务器的 running
	n, err := st.FailServerRuns(ctx, server, "Agent 连接中断")
	if err != nil || n != 1 {
		t.Fatalf("应清理 1 条: %v %d", err, n)
	}
	if rn, _ := st.GetRun(ctx, r1.ID); rn.Status != "failed" {
		t.Fatal("该服务器 run 应已失败")
	}
	runs, _ := st.ListRuns(ctx, 10)
	remainingRunning := 0
	for _, rn := range runs {
		if rn.Status == "running" {
			remainingRunning++
		}
	}
	if remainingRunning != 1 {
		t.Fatalf("另一服务器的 run 不应受影响: %d", remainingRunning)
	}

	// 启动清场：全部遗留 running 失败
	n, err = st.FailAllRunningRuns(ctx, "Hub 重启")
	if err != nil || n != 1 {
		t.Fatalf("清场数量异常: %v %d", err, n)
	}
}

func TestMigrationsAppliedV2(t *testing.T) {
	st := newTestStore(t)
	// 四张 M1 表都已建成（通过能正常 CRUD 佐证），此处校验迁移版本记录
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 2 {
		t.Fatalf("应有至少 2 个迁移版本，实际 %d", count)
	}
}
