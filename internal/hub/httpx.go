package hub

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]apiError{"error": {Code: code, Message: message}})
}

// decodeJSON 解析请求体；失败时已写入 400 响应，调用方直接返回。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "请求内容不是合法的 JSON")
		return false
	}
	return true
}

// clientIP 返回客户端 IP。仅当 trustProxy 为 true 时才采信 X-Forwarded-For，
// 并取最右一项——那是我们自己的反向代理追加的，左边的部分客户端可以伪造。
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			last := strings.TrimSpace(parts[len(parts)-1])
			if ip := net.ParseIP(last); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// originAllowed 拦截跨站的状态修改请求（CSRF 的第二道防线，第一道是 SameSite=Strict）。
// 浏览器的跨站 POST 一定带 Origin；curl 等非浏览器客户端不带，放行。
func originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false // 包括 "null"
	}
	return strings.EqualFold(u.Host, r.Host)
}

// bearerToken 从 Authorization 头里取出 Bearer token。
func bearerToken(header string) string {
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// isSecureRequest 判断请求是否经由 HTTPS 到达，用于设置 Cookie 的 Secure 属性。
// 伪造 X-Forwarded-Proto 只会让 Cookie 变得更严格，不构成风险。
func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// secureHeaders 给所有响应加上基础安全头。
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// csrfGuard 对所有修改类请求校验 Origin。
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !originAllowed(r) {
				writeError(w, http.StatusForbidden, "bad_origin", "请求来源不被允许")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
