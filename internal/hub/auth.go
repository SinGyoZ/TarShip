package hub

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName      = "tarship_token"
	settingUser     = "admin_username"
	settingPassHash = "admin_password_hash"
	jwtSecretFile   = "jwt.secret"
	tokenTTL        = 24 * time.Hour
)

var errUnauthorized = errors.New("未登录或会话已过期")

// authService 管理面板账号与 JWT 会话（bcrypt 哈希 + HttpOnly Cookie）。
type authService struct {
	store  *store
	secret []byte
}

// newAuth 读取或生成 JWT 签名密钥：优先环境变量 TARSHIP_JWT_SECRET，
// 否则落盘到数据目录（0600），重启后会话不失效。
func newAuth(store *store, dataDir string) (*authService, error) {
	if env := strings.TrimSpace(os.Getenv("TARSHIP_JWT_SECRET")); env != "" {
		// 与文档约定一致：过短的密钥会显著削弱 JWT 抗碰撞性
		if len(env) < 32 {
			return nil, errors.New("TARSHIP_JWT_SECRET 长度需至少 32 字节")
		}
		return &authService{store: store, secret: []byte(env)}, nil
	}
	path := filepath.Join(dataDir, jwtSecretFile)
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return &authService{store: store, secret: b}, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	hexStr := hex.EncodeToString(buf)
	// 目录可能不存在（首次启动），MkdirAll 保证密钥文件可落盘
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hexStr), 0o600); err != nil {
		return nil, err
	}
	return &authService{store: store, secret: []byte(hexStr)}, nil
}

// initialized 判断面板是否已完成首启初始化。
func (a *authService) initialized() bool {
	_, err := a.store.getSetting(settingUser)
	return err == nil
}

// setup 首启初始化：仅允许执行一次，之后返回错误。
func (a *authService) setup(username, password string) error {
	if a.initialized() {
		return errors.New("面板已初始化，请直接登录")
	}
	if err := validateCredentials(username, password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := a.store.putSetting(settingUser, username); err != nil {
		return err
	}
	return a.store.putSetting(settingPassHash, string(hash))
}

// login 校验账号密码，成功返回待签发的用户名。
func (a *authService) login(username, password string) (string, error) {
	savedUser, err := a.store.getSetting(settingUser)
	if err != nil {
		return "", errors.New("面板尚未初始化")
	}
	savedHash, err := a.store.getSetting(settingPassHash)
	if err != nil {
		return "", errors.New("账号数据损坏，请检查数据库")
	}
	if bcrypt.CompareHashAndPassword([]byte(savedHash), []byte(password)) != nil ||
		savedUser != username {
		return "", errors.New("用户名或密码错误")
	}
	return username, nil
}

// issueToken 签发 24 小时 JWT。
func (a *authService) issueToken(username string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   username,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.secret)
}

// parseToken 校验 JWT 并返回用户名。
func (a *authService) parseToken(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		// 签名算法必须显式等于 HS256，防止算法替换攻击
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非法签名算法")
		}
		return a.secret, nil
	})
	if err != nil || !token.Valid {
		return "", errUnauthorized
	}
	sub, err := token.Claims.GetSubject()
	if err != nil || sub == "" {
		return "", errUnauthorized
	}
	return sub, nil
}

// sessionUser 从请求 Cookie 中解析当前登录用户。
func (a *authService) sessionUser(r *http.Request) (string, error) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return "", errUnauthorized
	}
	return a.parseToken(c.Value)
}

// setSessionCookie 写入 HttpOnly 会话 Cookie（SameSite=Lax，防 CSRF 的基础防线）。
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(tokenTTL),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// validateCredentials 面板账号基础强度校验。
func validateCredentials(username, password string) error {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 32 {
		return errors.New("用户名长度需在 3-32 字符之间")
	}
	if len(password) < 8 || len(password) > 128 {
		return errors.New("密码长度需在 8-128 字符之间")
	}
	return nil
}
