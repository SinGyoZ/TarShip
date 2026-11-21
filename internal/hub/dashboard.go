package hub

import (
	"net/http"

	"golang.org/x/mod/semver"

	"github.com/SinGyoZ/TarShip/internal/version"
)

// isOutdated 用语义化版本比较判断 Agent 是否落后于 Hub（方案 4.4 的版本提示）。
// 任一方非 semver（如 dev 开发版）时不计入，避免字符串不等导致的误报。
func isOutdated(agentVersion, hubVersion string) bool {
	if !semver.IsValid(agentVersion) || !semver.IsValid(hubVersion) {
		return false
	}
	return semver.Compare(agentVersion, hubVersion) < 0
}

// handleDashboard 返回聚合统计：服务器在线概况与版本落后计数。
func (h *Hub) handleDashboard(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListServers(r.Context())
	if err != nil {
		h.internalError(w, "查询统计失败", err)
		return
	}
	online, outdated := 0, 0
	for _, s := range list {
		if ok, _ := h.presence.status(s.ID); !ok {
			continue
		}
		online++
		// presence 里持有的是 hello 上报的最新版本，比库里的 last_seen 快照更准确
		if h.presence.agentVersion(s.ID) != "" && isOutdated(h.presence.agentVersion(s.ID), version.Version) {
			outdated++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": version.Version,
		"servers": map[string]int{
			"total":    len(list),
			"online":   online,
			"offline":  len(list) - online,
			"outdated": outdated,
		},
	})
}
