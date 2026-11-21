package hub

// M1 端到端：真 Agent 在进程内连真 Hub，触发手动备份，
// 校验产物落盘、sha256 一致、排除规则生效、面板事件与 API 登记全部就位。

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupEndToEnd(t *testing.T) {
	e := newEnv(t)
	e.login()
	serverID, token := e.createServer("backup-host")
	stop, _ := e.startAgent(token)
	defer stop()

	// 被备份目录：正文 + 应被排除的文件
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "index.html"), []byte("<h1>tarship</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "app", "secret.env"), []byte("SKIP-ME"), 0o644); err != nil {
		t.Fatal(err)
	}

	// local 存储器（Agent 与存储器同机）
	storageDir := t.TempDir()
	var storage struct {
		ID int64 `json:"id"`
	}
	e.call("POST", "/api/storages", map[string]any{
		"name": "本机存储", "type": "local", "root_dir": storageDir,
	}, 201, &storage)

	// 任务：排除 secret.env
	var task struct {
		ID int64 `json:"id"`
	}
	e.call("POST", "/api/tasks", map[string]any{
		"name": "网站数据", "server_id": serverID, "storage_id": storage.ID,
		"paths": []string{srcDir}, "excludes": []string{"**/secret.env"},
	}, 201, &task)

	// 面板先连上，捕获 run_update 事件
	panel := e.dialPanel()

	var run struct {
		ID int64  `json:"id"`
		Status string `json:"status"`
	}
	e.call("POST", "/api/tasks/1/run", nil, 202, &run)
	if run.Status != "running" || run.ID == 0 {
		t.Fatalf("触发返回异常: %+v", run)
	}

	// 等面板收到成功的 run_update
	done := e.waitEvent(panel, func(ev map[string]any) bool {
		if ev["type"] != "run_update" {
			return false
		}
		rn, ok := ev["run"].(map[string]any)
		if !ok || int64(rn["id"].(float64)) != run.ID {
			return false
		}
		return rn["status"] == "success"
	})
	rn := done["run"].(map[string]any)
	objectKey := rn["object_key"].(string)
	size := int64(rn["size_bytes"].(float64))
	sha := rn["sha256"].(string)

	// 1) 产物真实落盘，大小一致
	product := filepath.Join(storageDir, objectKey)
	fi, err := os.Stat(product)
	if err != nil {
		t.Fatalf("产物未落盘: %v", err)
	}
	if fi.Size() != size {
		t.Fatalf("产物大小不一致: api=%d file=%d", size, fi.Size())
	}

	// 2) sha256 与文件内容一致
	raw, err := os.ReadFile(product)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != sha {
		t.Fatal("产物 sha256 与上报值不一致")
	}

	// 3) run 历史与备份产物登记
	var runs []runDTO
	e.call("GET", "/api/runs", nil, 200, &runs)
	if len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("运行历史异常: %+v", runs)
	}
	var backups []map[string]any
	e.call("GET", "/api/backups", nil, 200, &backups)
	if len(backups) != 1 {
		t.Fatalf("备份产物登记缺失: %v", backups)
	}
	if backups[0]["sha256"] != sha {
		t.Fatalf("登记的 sha256 不一致: %v", backups[0]["sha256"])
	}
}

func TestRunTaskAgentOfflineRejected(t *testing.T) {
	e := newEnv(t)
	e.login()
	serverID, _ := e.createServer("ghost") // 未连 Agent
	storageDir := t.TempDir()
	var storage struct {
		ID int64 `json:"id"`
	}
	e.call("POST", "/api/storages", map[string]any{"name": "s", "type": "local", "root_dir": storageDir}, 201, &storage)
	e.call("POST", "/api/tasks", map[string]any{
		"name": "t", "server_id": serverID, "storage_id": storage.ID, "paths": []string{"/data"},
	}, 201, nil)
	// Agent 离线：触发应 409，且不产生 run
	e.call("POST", "/api/tasks/1/run", nil, 409, nil)
	var runs []map[string]any
	e.call("GET", "/api/runs", nil, 200, &runs)
	if len(runs) != 0 {
		t.Fatalf("离线触发不应产生 run: %v", runs)
	}
}

func TestStorageSecretNotLeaked(t *testing.T) {
	e := newEnv(t)
	e.login()
	e.call("POST", "/api/storages", map[string]any{
		"name": "r2", "type": "s3", "endpoint": "e.r2.dev", "bucket": "b",
		"access_key_id": "ak", "secret_access_key": "super-secret-value",
	}, 201, nil)

	// 列表不回显 secret
	body := string(e.rawBody("GET", "/api/storages"))
	if contains(body, "super-secret-value") {
		t.Fatal("存储器列表泄露了 secret")
	}

	// 更新时 secret 留空 = 沿用旧值
	e.call("PUT", "/api/storages/1", map[string]any{
		"name": "r2-new", "type": "s3", "endpoint": "e.r2.dev", "bucket": "b", "access_key_id": "ak",
	}, 200, nil)
	var list []map[string]any
	e.call("GET", "/api/storages", nil, 200, &list)
	if len(list) != 1 || list[0]["name"] != "r2-new" {
		t.Fatalf("更新后列表异常: %v", list)
	}
}

// rawBody 请求并返回原始响应体。
func (e *testEnv) rawBody(method, path string) []byte {
	e.t.Helper()
	req, err := http.NewRequest(method, e.ts.URL+path, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		e.t.Fatalf("%s %s = %d", method, path, resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
