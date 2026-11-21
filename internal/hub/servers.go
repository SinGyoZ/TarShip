package hub

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/SinGyoZ/TarShip/internal/proto"
	"github.com/SinGyoZ/TarShip/internal/store"
)

const tokenPrefix = "tsa_" // 固定前缀便于密钥扫描工具识别泄露的 token

type serverDTO struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Hostname     string   `json:"hostname"`
	AgentVersion string   `json:"agent_version"`
	Tags         []string `json:"tags"`
	Online       bool     `json:"online"`
	LastSeenAt   *string  `json:"last_seen_at"`
	CreatedAt    string   `json:"created_at"`
}

func newAgentToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

// hashToken 返回 token 的 SHA-256 十六进制串。token 本身是 256 位随机数，不需要加盐或慢哈希。
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (h *Hub) toDTO(s store.Server) serverDTO {
	online, seen := h.presence.status(s.ID)
	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}
	dto := serverDTO{
		ID: s.ID, Name: s.Name, Hostname: s.Hostname, AgentVersion: s.AgentVersion,
		Tags: tags, Online: online, CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	last := s.LastSeenAt
	if online {
		last = &seen
	}
	if last != nil {
		str := last.UTC().Format(time.RFC3339Nano)
		dto.LastSeenAt = &str
	}
	return dto
}

func (h *Hub) listDTOs(ctx context.Context) ([]serverDTO, error) {
	list, err := h.store.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]serverDTO, 0, len(list))
	for _, s := range list {
		out = append(out, h.toDTO(s))
	}
	return out, nil
}

func (h *Hub) internalError(w http.ResponseWriter, msg string, err error) {
	h.log.Error(msg, "error", err)
	writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
}

func (h *Hub) handleListServers(w http.ResponseWriter, r *http.Request) {
	list, err := h.listDTOs(r.Context())
	if err != nil {
		h.internalError(w, "查询服务器列表失败", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Hub) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 64 {
		writeError(w, http.StatusBadRequest, "invalid_name", "服务器名称需为 1-64 个字符")
		return
	}
	tags, ok := cleanTags(req.Tags)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_tags", "标签最多 10 个，每个不超过 32 个字符")
		return
	}
	token, hash, err := newAgentToken()
	if err != nil {
		h.internalError(w, "生成 token 失败", err)
		return
	}
	sv, err := h.store.CreateServer(r.Context(), name, tags, hash)
	if err != nil {
		h.internalError(w, "创建服务器失败", err)
		return
	}
	h.publishServersChanged()
	// token 明文只在这一次响应里出现，库里只有哈希。
	writeJSON(w, http.StatusCreated, map[string]any{"server": h.toDTO(sv), "token": token})
}

func cleanTags(in []string) ([]string, bool) {
	if len(in) > 10 {
		return nil, false
	}
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > 32 {
			return nil, false
		}
		out = append(out, t)
	}
	return out, true
}

func parseServerID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (h *Hub) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	id, ok := parseServerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "服务器 ID 不合法")
		return
	}
	deleted, err := h.store.DeleteServer(r.Context(), id)
	if err != nil {
		h.internalError(w, "删除服务器失败", err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "not_found", "服务器不存在")
		return
	}
	h.presence.kick(id, websocket.StatusCode(proto.CloseRevoked), "服务器已被删除")
	h.publishServersChanged()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Hub) handleResetToken(w http.ResponseWriter, r *http.Request) {
	id, ok := parseServerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_id", "服务器 ID 不合法")
		return
	}
	token, hash, err := newAgentToken()
	if err != nil {
		h.internalError(w, "生成 token 失败", err)
		return
	}
	updated, err := h.store.UpdateServerToken(r.Context(), id, hash)
	if err != nil {
		h.internalError(w, "重置 token 失败", err)
		return
	}
	if !updated {
		writeError(w, http.StatusNotFound, "not_found", "服务器不存在")
		return
	}
	h.presence.kick(id, websocket.StatusCode(proto.CloseRevoked), "token 已被重置")
	sv, found, err := h.store.GetServer(r.Context(), id)
	if err != nil || !found {
		h.internalError(w, "读取服务器失败", err)
		return
	}
	h.publishServersChanged()
	writeJSON(w, http.StatusOK, map[string]any{"server": h.toDTO(sv), "token": token})
}
