package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Server 与方案第六章 servers 表对应；时间为 RFC3339 文本。
type Server struct {
	ID           int64
	Name         string
	Hostname     string
	AgentVersion string
	Tags         string
	LastSeenAt   string
	CreatedAt    string
}

// store 封装 Hub 对 SQLite 的全部访问，避免 SQL 散落各处。
type store struct {
	db *sql.DB
}

var errNotFound = errors.New("record not found")

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// newServerToken 生成 Agent 注册 token（tsk_ 前缀 + 32 字节随机）并返回明文与 SHA-256 摘要。
// 明文只在创建/重置时返回一次，库中只存摘要（方案第九章）。
func newServerToken() (plain, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("生成 token: %w", err)
	}
	plain = "tsk_" + base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(sum[:]), nil
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func scanServer(row interface{ Scan(...any) error }) (Server, error) {
	var s Server
	var hostname, agentVer, tags, lastSeen sql.NullString
	err := row.Scan(&s.ID, &s.Name, &hostname, &agentVer, &tags, &lastSeen, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s, errNotFound
		}
		return s, err
	}
	s.Hostname, s.AgentVersion, s.Tags, s.LastSeenAt =
		hostname.String, agentVer.String, tags.String, lastSeen.String
	return s, nil
}

const serverCols = `id, name, hostname, agent_version, tags, last_seen_at, created_at`

func (st *store) createServer(name, tokenHash, tags string) (Server, error) {
	res, err := st.db.Exec(
		`INSERT INTO servers (name, token_hash, tags, created_at) VALUES (?, ?, ?, ?)`,
		name, tokenHash, tags, nowRFC3339())
	if err != nil {
		return Server{}, fmt.Errorf("插入服务器: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Server{}, fmt.Errorf("取自增 ID: %w", err)
	}
	return st.getServer(id)
}

func (st *store) getServer(id int64) (Server, error) {
	row := st.db.QueryRow(`SELECT `+serverCols+` FROM servers WHERE id = ?`, id)
	return scanServer(row)
}

// getServerByTokenHash 供 Agent 建连认证使用。
func (st *store) getServerByTokenHash(hash string) (Server, error) {
	row := st.db.QueryRow(`SELECT `+serverCols+` FROM servers WHERE token_hash = ?`, hash)
	return scanServer(row)
}

func (st *store) listServers() ([]Server, error) {
	rows, err := st.db.Query(`SELECT ` + serverCols + ` FROM servers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("查询服务器列表: %w", err)
	}
	defer rows.Close()
	var out []Server
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// deleteServer 删除服务器注册记录；对应在线连接由 AgentHub 踢除。
func (st *store) deleteServer(id int64) error {
	res, err := st.db.Exec(`DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除服务器: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

// resetServerToken 重置 token：旧 Agent 立即失联，返回仅展示一次的新 token。
func (st *store) resetServerToken(id int64) (plain string, err error) {
	plain, hash, err := newServerToken()
	if err != nil {
		return "", err
	}
	res, err := st.db.Exec(`UPDATE servers SET token_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return "", fmt.Errorf("重置 token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", errNotFound
	}
	return plain, nil
}

// agentSeen 由 hello 与 heartbeat 上报时刷新，作为离线后的"最后心跳"展示。
func (st *store) agentSeen(id int64, hostname, agentVersion string) error {
	_, err := st.db.Exec(
		`UPDATE servers SET hostname = ?, agent_version = ?, last_seen_at = ? WHERE id = ?`,
		hostname, agentVersion, nowRFC3339(), id)
	return err
}

// getSetting / putSetting 读写 settings 键值表。
func (st *store) getSetting(key string) (string, error) {
	var v string
	err := st.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	return v, err
}

func (st *store) putSetting(key, value string) error {
	_, err := st.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// sanitizeName 校验面板输入，去掉首尾空白并限长，防止空名与异常长字符串入库。
func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}
