package hub

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.9:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.7")

	if got := clientIP(r, false); got != "10.0.0.9" {
		t.Errorf("不信任代理时应取 RemoteAddr, got %q", got)
	}
	if got := clientIP(r, true); got != "203.0.113.7" {
		t.Errorf("信任代理时应取最右一项, got %q", got)
	}
	r.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := clientIP(r, true); got != "10.0.0.9" {
		t.Errorf("XFF 非法时应回退 RemoteAddr, got %q", got)
	}
}

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		name          string
		origin, fetch string
		want          bool
	}{
		{"同源", "http://hub.example.com", "", true},
		{"大小写不敏感", "http://HUB.example.com", "", true},
		{"跨站", "http://evil.example", "", false},
		{"null", "null", "", false},
		{"无 Origin 的非浏览器客户端", "", "", true},
		{"无 Origin 但 Sec-Fetch-Site 为跨站", "", "cross-site", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "http://hub.example.com/api/x", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.fetch != "" {
			r.Header.Set("Sec-Fetch-Site", c.fetch)
		}
		if got := originAllowed(r); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]string{
		"Bearer abc": "abc",
		"bearer abc": "abc",
		"Bearer ":    "",
		"Basic abc":  "",
		"":           "",
		"Bearerabc":  "",
	}
	for in, want := range cases {
		if got := bearerToken(in); got != want {
			t.Errorf("bearerToken(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestCSRFGuard(t *testing.T) {
	h := csrfGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for method, want := range map[string]int{"GET": 204, "POST": 403, "DELETE": 403} {
		r := httptest.NewRequest(method, "http://hub.example.com/api/x", nil)
		r.Header.Set("Origin", "http://evil.example")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("%s 跨站请求: got %d want %d", method, w.Code, want)
		}
	}
}

func TestCleanText(t *testing.T) {
	if got := cleanText("  ho\nst\x00name ", 255); got != "hostname" {
		t.Errorf("got %q", got)
	}
	if got := cleanText("你好世界", 2); got != "你好" {
		t.Errorf("应按字符而非字节截断, got %q", got)
	}
}
