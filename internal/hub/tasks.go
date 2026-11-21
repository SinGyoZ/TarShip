// hub 任务与运行：任务 CRUD、手动备份触发、运行历史与 Agent 上报结算。
package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/SinGyoZ/TarShip/internal/proto"
	"github.com/SinGyoZ/TarShip/internal/store"
)

// taskDTO 是任务对外形态。
type taskDTO struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	ServerID      int64    `json:"server_id"`
	StorageID     int64    `json:"storage_id"`
	Paths         []string `json:"paths"`
	Excludes      []string `json:"excludes"`
	CronExpr      string   `json:"cron_expr,omitempty"`
	RetentionDays int      `json:"retention_days,omitempty"`
	Enabled       bool     `json:"enabled"`
	CreatedAt     string   `json:"created_at"`
}

func (h *Hub) toTaskDTO(tk store.Task) taskDTO {
	return taskDTO{
		ID: tk.ID, Name: tk.Name, ServerID: tk.ServerID, StorageID: tk.StorageID,
		Paths: tk.Paths, Excludes: tk.Excludes, CronExpr: tk.CronExpr,
		RetentionDays: tk.RetentionDays, Enabled: tk.Enabled,
		CreatedAt: tk.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

type taskUpsert struct {
	Name          string   `json:"name"`
	ServerID      int64    `json:"server_id"`
	StorageID     int64    `json:"storage_id"`
	Paths         []string `json:"paths"`
	Excludes      []string `json:"excludes"`
	CronExpr      string   `json:"cron_expr"`
	RetentionDays int      `json:"retention_days"`
	Enabled        *bool   `json:"enabled"`
}

func (h *Hub) handleListTasks(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListTasks(r.Context())
	if err != nil {
		h.internalError(w, "查询任务失败", err)
		return
	}
	out := make([]taskDTO, 0, len(list))
	for _, tk := range list {
		out = append(out, h.toTaskDTO(tk))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Hub) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	tk, ok := h.decodeTask(w, r)
	if !ok {
		return
	}
	if err := h.checkTaskRefs(r.Context(), tk); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ref", err.Error())
		return
	}
	created, err := h.store.CreateTask(r.Context(), tk)
	if err != nil {
		h.internalError(w, "创建任务失败", err)
		return
	}
	h.log.Info("任务已创建", "id", created.ID, "name", created.Name)
	writeJSON(w, http.StatusCreated, h.toTaskDTO(created))
}

func (h *Hub) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseServerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "任务 ID 不合法")
		return
	}
	tk, ok := h.decodeTask(w, r)
	if !ok {
		return
	}
	tk.ID = id
	if err := h.checkTaskRefs(r.Context(), tk); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ref", err.Error())
		return
	}
	updated, err := h.store.UpdateTask(r.Context(), id, tk)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if err != nil {
		h.internalError(w, "更新任务失败", err)
		return
	}
	writeJSON(w, http.StatusOK, h.toTaskDTO(updated))
}

// decodeTask 解析并校验任务请求体。
func (h *Hub) decodeTask(w http.ResponseWriter, r *http.Request) (store.Task, bool) {
	var req taskUpsert
	if !decodeJSON(w, r, &req) {
		return store.Task{}, false
	}
	tk := store.Task{
		Name: req.Name, ServerID: req.ServerID, StorageID: req.StorageID,
		Paths: req.Paths, Excludes: req.Excludes,
		CronExpr: req.CronExpr, RetentionDays: req.RetentionDays, Enabled: true,
	}
	if req.Enabled != nil {
		tk.Enabled = *req.Enabled
	}
	if err := store.ValidateTask(&tk); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_task", err.Error())
		return store.Task{}, false
	}
	return tk, true
}

// checkTaskRefs 校验任务引用的服务器与存储器存在。
func (h *Hub) checkTaskRefs(ctx context.Context, tk store.Task) error {
	if _, _, err := h.store.GetServer(ctx, tk.ServerID); err != nil {
		return errors.New("目标服务器不存在")
	}
	if _, err := h.store.GetStorage(ctx, tk.StorageID); err != nil {
		return errors.New("存储器不存在")
	}
	return nil
}

func (h *Hub) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseServerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "任务 ID 不合法")
		return
	}
	deleted, err := h.store.DeleteTask(r.Context(), id)
	if err != nil {
		h.internalError(w, "删除任务失败", err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	h.publishTasksChanged()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- 运行 ----

// runDTO 是运行对外形态；running 状态附带 Agent 实时上报的进度。
type runDTO struct {
	ID            int64  `json:"id"`
	TaskID        int64  `json:"task_id"`
	ServerID      int64  `json:"server_id"`
	StorageID     int64  `json:"storage_id"`
	Status        string `json:"status"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	UploadedBytes int64  `json:"uploaded_bytes,omitempty"`
	Sha256        string `json:"sha256,omitempty"`
	ObjectKey     string `json:"object_key,omitempty"`
	ErrorMsg      string `json:"error_msg,omitempty"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at,omitempty"`
}

func (h *Hub) toRunDTO(rn store.Run) runDTO {
	dto := runDTO{
		ID: rn.ID, TaskID: rn.TaskID, ServerID: rn.ServerID, StorageID: rn.StorageID,
		Status: rn.Status, SizeBytes: rn.SizeBytes, Sha256: rn.Sha256,
		ObjectKey: rn.ObjectKey, ErrorMsg: rn.ErrorMsg,
		StartedAt: rn.StartedAt.UTC().Format(time.RFC3339Nano),
	}
	if rn.FinishedAt != nil {
		dto.FinishedAt = rn.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	if rn.Status == "running" {
		// running 时用内存进度补齐实时字节数
		if v, ok := h.runProgress.Load(rn.ID); ok {
			dto.UploadedBytes = v.(int64)
		}
	}
	return dto
}

// handleRunTask 触发一次手动备份：创建 run → 下发指令 → 立即返回，结果经面板事件推送。
func (h *Hub) handleRunTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseServerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "任务 ID 不合法")
		return
	}
	tk, err := h.store.GetTask(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if err != nil {
		h.internalError(w, "读取任务失败", err)
		return
	}
	if _, online := h.presence.session(tk.ServerID); !online {
		writeError(w, http.StatusConflict, "agent_offline", "目标服务器 Agent 离线，无法备份")
		return
	}
	st, err := h.store.GetStorage(r.Context(), tk.StorageID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, "invalid_ref", "任务引用的存储器已被删除")
		return
	}
	if err != nil {
		h.internalError(w, "读取存储器失败", err)
		return
	}
	rn, err := h.store.CreateRun(r.Context(), tk.ID, tk.ServerID, tk.StorageID)
	if err != nil {
		writeError(w, http.StatusConflict, "already_running", err.Error())
		return
	}
	err = h.presence.sendToServer(r.Context(), tk.ServerID, proto.TaskBackup{
		Type: proto.TypeTaskBackup, RunID: rn.ID, TaskID: tk.ID, ServerID: tk.ServerID,
		Paths: tk.Paths, Excludes: tk.Excludes, Storage: storageTargetOf(st),
	})
	if err != nil {
		_ = h.store.FinishRunFailed(r.Context(), rn.ID, "下发备份指令失败: "+err.Error())
		h.internalError(w, "下发备份指令失败", err)
		return
	}
	h.log.Info("手动备份已触发", "run_id", rn.ID, "task", tk.Name)
	h.publishRun(rn.ID)
	writeJSON(w, http.StatusAccepted, h.toRunDTO(rn))
}

func (h *Hub) handleListRuns(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListRuns(r.Context(), 50)
	if err != nil {
		h.internalError(w, "查询运行历史失败", err)
		return
	}
	out := make([]runDTO, 0, len(list))
	for _, rn := range list {
		out = append(out, h.toRunDTO(rn))
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- Agent 上报结算 ----

// handleRunProgress 记录内存进度并推面板；不落库，避免高频写放大。
func (h *Hub) handleRunProgress(data []byte) {
	var msg proto.RunProgress
	if err := json.Unmarshal(data, &msg); err != nil || msg.RunID <= 0 {
		return
	}
	h.runProgress.Store(msg.RunID, msg.UploadedBytes)
	h.panel.publish(map[string]any{
		"type": "run_update", "run_id": msg.RunID, "status": "running",
		"uploaded_bytes": msg.UploadedBytes,
	})
}

// handleRunDone 结算成功：落库 + 登记产物 + 清进度缓存。
func (h *Hub) handleRunDone(data []byte) {
	var msg proto.RunDone
	if err := json.Unmarshal(data, &msg); err != nil || msg.RunID <= 0 {
		return
	}
	h.dbCall(func(ctx context.Context) error {
		return h.store.FinishRunSuccess(ctx, msg.RunID, msg.ObjectKey, msg.SizeBytes, msg.Sha256)
	})
	h.runProgress.Delete(msg.RunID)
	h.log.Info("备份运行成功", "run_id", msg.RunID, "size", msg.SizeBytes)
	h.publishRun(msg.RunID)
}

// handleRunFailed 结算失败。
func (h *Hub) handleRunFailed(data []byte) {
	var msg proto.RunFailed
	if err := json.Unmarshal(data, &msg); err != nil || msg.RunID <= 0 {
		return
	}
	h.dbCall(func(ctx context.Context) error {
		return h.store.FinishRunFailed(ctx, msg.RunID, msg.Message)
	})
	h.runProgress.Delete(msg.RunID)
	h.log.Warn("备份运行失败", "run_id", msg.RunID, "error", msg.Message)
	h.publishRun(msg.RunID)
}

// failServerRunningRuns 在 Agent 断线时把其未完成的运行标记失败，防止 run 永久卡在 running。
func (h *Hub) failServerRunningRuns(serverID int64) {
	var affected int64
	h.dbCall(func(ctx context.Context) error {
		var err error
		affected, err = h.store.FailServerRuns(ctx, serverID, "Agent 连接中断，运行被终止")
		return err
	})
	if affected > 0 {
		h.log.Warn("Agent 断线，运行已终止", "server_id", serverID, "runs", affected)
		h.publishRunsChanged()
	}
}

// failStaleRuns 在 Hub 启动时清场：上次进程退出遗留的 running 一律失败。
func (h *Hub) failStaleRuns() {
	var affected int64
	h.dbCall(func(ctx context.Context) error {
		var err error
		affected, err = h.store.FailAllRunningRuns(context.Background(), "Hub 重启，运行被终止")
		return err
	})
	if affected > 0 {
		h.log.Warn("清理上次遗留的运行记录", "runs", affected)
	}
}

// publishRun 推送某次运行的最新状态；面板据此失效 runs/backups 缓存。
func (h *Hub) publishRun(runID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rn, err := h.store.GetRun(ctx, runID)
	if err != nil {
		return
	}
	h.panel.publish(map[string]any{"type": "run_update", "run": h.toRunDTO(rn)})
}

func (h *Hub) publishRunsChanged() {
	h.panel.publish(map[string]string{"type": "runs_changed"})
}

func (h *Hub) publishTasksChanged() {
	h.panel.publish(map[string]string{"type": "tasks_changed"})
}

// ---- 备份产物 ----

func (h *Hub) handleListBackups(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListBackups(r.Context(), 100)
	if err != nil {
		h.internalError(w, "查询备份产物失败", err)
		return
	}
	type backupDTO struct {
		ID        int64  `json:"id"`
		RunID     int64  `json:"run_id"`
		TaskID    int64  `json:"task_id"`
		ServerID  int64  `json:"server_id"`
		ObjectKey string `json:"object_key"`
		SizeBytes int64  `json:"size_bytes"`
		Sha256    string `json:"sha256"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]backupDTO, 0, len(list))
	for _, b := range list {
		out = append(out, backupDTO{
			ID: b.ID, RunID: b.RunID, TaskID: b.TaskID, ServerID: b.ServerID,
			ObjectKey: b.ObjectKey, SizeBytes: b.SizeBytes, Sha256: b.Sha256,
			CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
