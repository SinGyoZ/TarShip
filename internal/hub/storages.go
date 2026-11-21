// hub 存储器管理：CRUD API、凭据脱敏与连通性测试。
// s3 测试由 Hub 直接 HeadBucket；local 目录在被备份机器上，指令下发 Agent 实测可写性。
package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/SinGyoZ/TarShip/internal/proto"
	"github.com/SinGyoZ/TarShip/internal/store"
)

// storageDTO 是存储器对外形态；secret 一律不出明文（仅创建/更新请求里短暂携带）。
type storageDTO struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	RootDir   string   `json:"root_dir,omitempty"`
	Endpoint  string   `json:"endpoint,omitempty"`
	Region    string   `json:"region,omitempty"`
	Bucket    string   `json:"bucket,omitempty"`
	AccessKeyID string `json:"access_key_id,omitempty"`
	Prefix    string   `json:"prefix,omitempty"`
	CreatedAt string   `json:"created_at"`
}

func (h *Hub) toStorageDTO(st store.Storage) storageDTO {
	return storageDTO{
		ID: st.ID, Name: st.Name, Type: st.Type,
		RootDir: st.RootDir, Endpoint: st.Endpoint, Region: st.Region,
		Bucket: st.Bucket, AccessKeyID: st.AccessKeyID, Prefix: st.Prefix,
		CreatedAt: st.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// storageUpsert 是创建/更新的请求体；Type 决定其余哪些字段生效。
type storageUpsert struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	RootDir         string `json:"root_dir"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Prefix          string `json:"prefix"`
}

func (r storageUpsert) toModel() store.Storage {
	return store.Storage{
		Name: r.Name, Type: r.Type, RootDir: strings.TrimSpace(r.RootDir),
		Endpoint: strings.TrimSpace(r.Endpoint), Region: strings.TrimSpace(r.Region),
		Bucket: strings.TrimSpace(r.Bucket), AccessKeyID: strings.TrimSpace(r.AccessKeyID),
		SecretAccessKey: r.SecretAccessKey, Prefix: r.Prefix,
	}
}

func (h *Hub) handleListStorages(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListStorages(r.Context())
	if err != nil {
		h.internalError(w, "查询存储器失败", err)
		return
	}
	out := make([]storageDTO, 0, len(list))
	for _, st := range list {
		out = append(out, h.toStorageDTO(st))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Hub) handleCreateStorage(w http.ResponseWriter, r *http.Request) {
	var req storageUpsert
	if !decodeJSON(w, r, &req) {
		return
	}
	st := req.toModel()
	if err := store.ValidateStorage(&st); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_storage", err.Error())
		return
	}
	created, err := h.store.CreateStorage(r.Context(), st)
	if err != nil {
		h.internalError(w, "创建存储器失败", err)
		return
	}
	h.log.Info("存储器已创建", "id", created.ID, "name", created.Name, "type", created.Type)
	writeJSON(w, http.StatusCreated, h.toStorageDTO(created))
}

func (h *Hub) handleUpdateStorage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseServerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "存储器 ID 不合法")
		return
	}
	var req storageUpsert
	if !decodeJSON(w, r, &req) {
		return
	}
	old, err := h.store.GetStorage(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "存储器不存在")
		return
	}
	if err != nil {
		h.internalError(w, "读取存储器失败", err)
		return
	}
	st := req.toModel()
	// secret 为空表示沿用旧值：前端不回显明文，避免误清空凭据
	if st.SecretAccessKey == "" {
		st.SecretAccessKey = old.SecretAccessKey
	}
	if err := store.ValidateStorage(&st); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_storage", err.Error())
		return
	}
	updated, err := h.store.UpdateStorage(r.Context(), id, st)
	if err != nil {
		h.internalError(w, "更新存储器失败", err)
		return
	}
	writeJSON(w, http.StatusOK, h.toStorageDTO(updated))
}

func (h *Hub) handleDeleteStorage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseServerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "存储器 ID 不合法")
		return
	}
	deleted, err := h.store.DeleteStorage(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "in_use", err.Error())
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "not_found", "存储器不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// storageTestTimeout 是等待 Agent 回报 local 可写性测试的上限。
const storageTestTimeout = 15 * time.Second

// handleTestStorage 发起连通性测试：s3 由 Hub 直接 HeadBucket；
// local 必须在被备份机器上验证，请求体需指定 server_id，结果异步等 Agent 回报。
func (h *Hub) handleTestStorage(w http.ResponseWriter, r *http.Request) {
	id, ok := parseServerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "存储器 ID 不合法")
		return
	}
	st, err := h.store.GetStorage(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "存储器不存在")
		return
	}
	if err != nil {
		h.internalError(w, "读取存储器失败", err)
		return
	}

	switch st.Type {
	case "s3":
		h.testS3(w, r, st)
	case "local":
		h.testLocal(w, r, st)
	default:
		writeError(w, http.StatusBadRequest, "invalid_storage", "未知存储器类型")
	}
}

func (h *Hub) testS3(w http.ResponseWriter, r *http.Request, st store.Storage) {
	if err := h.checkBucket(r.Context(), st); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "bucket 可访问"})
}

func (h *Hub) checkBucket(ctx context.Context, st store.Storage) error {
	client, err := minio.New(st.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(st.AccessKeyID, st.SecretAccessKey, st.Region),
		Secure: !strings.HasPrefix(strings.ToLower(st.Endpoint), "http://"),
	})
	if err != nil {
		return err
	}
	exists, err := client.BucketExists(ctx, st.Bucket)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("bucket 不存在或无访问权限")
	}
	return nil
}

func (h *Hub) testLocal(w http.ResponseWriter, r *http.Request, st store.Storage) {
	var req struct {
		ServerID int64 `json:"server_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ServerID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_server", "local 存储器需要在指定服务器上测试，请选择服务器")
		return
	}
	testID, err := randomHex(8)
	if err != nil {
		h.internalError(w, "生成测试 ID 失败", err)
		return
	}
	ch := make(chan proto.StorageTestDone, 1)
	h.pendingTests.Store(testID, ch)
	defer h.pendingTests.Delete(testID)

	err = h.presence.sendToServer(r.Context(), req.ServerID, proto.StorageTest{
		Type: proto.TypeStorageTest, TestID: testID,
		RootDir: st.RootDir, Storage: storageTargetOf(st),
	})
	if errors.Is(err, errAgentOffline) {
		writeError(w, http.StatusConflict, "agent_offline", "该服务器 Agent 离线，无法测试 local 目录")
		return
	}
	if err != nil {
		h.internalError(w, "下发测试指令失败", err)
		return
	}
	select {
	case done := <-ch:
		writeJSON(w, http.StatusOK, map[string]any{"ok": done.OK, "message": done.Message})
	case <-time.After(storageTestTimeout):
		writeError(w, http.StatusGatewayTimeout, "test_timeout", "等待 Agent 测试结果超时")
	case <-r.Context().Done():
		// 客户端取消等结果时清理 pending 记录，避免协程泄漏
		return
	}
}

func storageTargetOf(st store.Storage) proto.StorageTarget {
	return proto.StorageTarget{
		Type: st.Type, RootDir: st.RootDir, Endpoint: st.Endpoint, Region: st.Region,
		Bucket: st.Bucket, AccessKeyID: st.AccessKeyID,
		SecretAccessKey: st.SecretAccessKey, Prefix: st.Prefix,
	}
}


// ---- Agent 回报路由 ----

// handleStorageTestDone 把 Agent 的测试结果送回等待中的 HTTP 请求。
func (h *Hub) handleStorageTestDone(data []byte) {
	var done proto.StorageTestDone
	if err := json.Unmarshal(data, &done); err != nil {
		return
	}
	if ch, ok := h.pendingTests.Load(done.TestID); ok {
		ch.(chan proto.StorageTestDone) <- done
	}
}

