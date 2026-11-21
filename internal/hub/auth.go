package hub

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/SinGyoZ/TarShip/internal/store"
)

const (
	cookieName   = "tarship_session"
	sessionTTL   = 24 * time.Hour
	keyJWTSecret = "jwt_secret"

	maxLoginFails = 5
	loginWindow   = 10 * time.Minute
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

type ctxKey struct{}

type authService struct {
	store      *store.Store
	log        *slog.Logger
	secret     []byte
	trustProxy bool
	limiter    *limiter
	// 用户名不存在时也对它做一次 bcrypt 比较，避免响应时间泄露用户名是否正确。
	dummyHash []byte

	mu         sync.Mutex
	setupToken string // 仅在管理员尚未创建时非空
}

func newAuthService(ctx context.Context, st *store.Store, log *slog.Logger, trustProxy bool) (*authService, error) {
	// 环境变量优先（至少 32 字节）：便于运维统一管理密钥与多实例部署场景；
	// 未设置时首次启动自动生成并落库，重启后会话不失效。
	var secret string
	if env := strings.TrimSpace(os.Getenv("TARSHIP_JWT_SECRET")); env != "" {
		if len(env) < 32 {
			return nil, errors.New("TARSHIP_JWT_SECRET 长度需至少 32 字节")
		}
		secret = env
	} else {
		var err error
		secret, err = st.EnsureSetting(ctx, keyJWTSecret, func() (string, error) { return randomHex(32) })
		if err != nil {
			return nil, err
		}
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("tarship-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	a := &authService{
		store:      st,
		log:        log,
		secret:     []byte(secret),
		trustProxy: trustProxy,
		limiter:    newLimiter(maxLoginFails, loginWindow),
		dummyHash:  dummy,
	}
	_, _, adminExists, err := st.Admin(ctx)
	if err != nil {
		return nil, err
	}
	if !adminExists {
		tok, err := randomHex(16)
		if err != nil {
			return nil, err
		}
		a.setupToken = tok
		// 公网部署时避免管理员账号被抢注：只有看过日志的人才拿得到这个一次性 token。
		log.Info("首次启动：请打开面板，用下面的一次性 setup token 创建管理员账号", "setup_token", tok)
	}
	return a, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (a *authService) currentSetupToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.setupToken
}

func (a *authService) clearSetupToken() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setupToken = ""
}

func (a *authService) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"initialized": a.currentSetupToken() == ""})
}

func (a *authService) handleSetup(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r, a.trustProxy)
	if ok, retry := a.limiter.allow(ip); !ok {
		tooMany(w, retry)
		return
	}
	var req struct {
		SetupToken string `json:"setup_token"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	tok := a.currentSetupToken()
	if tok == "" {
		writeError(w, http.StatusConflict, "already_initialized", "管理员账号已经创建，请直接登录")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.SetupToken), []byte(tok)) != 1 {
		a.limiter.fail(ip)
		writeError(w, http.StatusForbidden, "invalid_setup_token", "setup token 不正确，请查看 Hub 启动日志")
		return
	}
	if !usernamePattern.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "invalid_username", "用户名需为 3-32 位字母、数字、下划线、点或连字符")
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 72 { // bcrypt 只处理前 72 字节
		writeError(w, http.StatusBadRequest, "invalid_password", "密码长度需在 8-72 字节之间")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		a.log.Error("生成密码哈希失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	created, err := a.store.CreateAdmin(r.Context(), req.Username, string(hash))
	if err != nil {
		a.log.Error("创建管理员失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	if !created {
		a.clearSetupToken()
		writeError(w, http.StatusConflict, "already_initialized", "管理员账号已经创建，请直接登录")
		return
	}
	a.clearSetupToken()
	if err := a.issueSession(w, r, req.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	a.log.Info("管理员账号已创建", "username", req.Username)
	writeJSON(w, http.StatusOK, map[string]string{"username": req.Username})
}

func (a *authService) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r, a.trustProxy)
	if ok, retry := a.limiter.allow(ip); !ok {
		tooMany(w, retry)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, hash, exists, err := a.store.Admin(r.Context())
	if err != nil {
		a.log.Error("读取管理员失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	hashBytes, userOK := a.dummyHash, false
	if exists {
		hashBytes = []byte(hash)
		userOK = subtle.ConstantTimeCompare([]byte(req.Username), []byte(user)) == 1
	}
	pwOK := bcrypt.CompareHashAndPassword(hashBytes, []byte(req.Password)) == nil
	if !exists || !userOK || !pwOK {
		a.limiter.fail(ip)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	a.limiter.reset(ip)
	if err := a.issueSession(w, r, user); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "服务器内部错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": user})
}

func (a *authService) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: isSecureRequest(r),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *authService) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": usernameFrom(r.Context())})
}

func tooMany(w http.ResponseWriter, retry time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	writeError(w, http.StatusTooManyRequests, "too_many_attempts", "失败次数过多，请稍后再试")
}

func (a *authService) issueSession(w http.ResponseWriter, r *http.Request, username string) error {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   username,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(sessionTTL)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.secret)
	if err != nil {
		a.log.Error("签发会话失败", "error", err)
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: signed, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: isSecureRequest(r),
	})
	return nil
}

func (a *authService) parseSession(r *http.Request) (string, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", err
	}
	claims := &jwt.RegisteredClaims{}
	_, err = jwt.ParseWithClaims(c.Value, claims,
		func(*jwt.Token) (any, error) { return a.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return "", err
	}
	if claims.Subject == "" {
		return "", errors.New("会话缺少 subject")
	}
	return claims.Subject, nil
}

// require 保护需要登录的路由。
func (a *authService) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := a.parseSession(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	})
}

func usernameFrom(ctx context.Context) string {
	u, _ := ctx.Value(ctxKey{}).(string)
	return u
}
