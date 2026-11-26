// Package store 封装 SQLite 访问：迁移、服务器表、设置表。
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const timeLayout = time.RFC3339Nano

// Store 是对 *sql.DB 的薄封装。
type Store struct {
	db *sql.DB
}

// Open 在 dataDir 下打开（必要时创建）数据库并执行迁移。
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录 %s 失败（可用 --data 指定其他目录）: %w", dataDir, err)
	}
	if err := probeWritable(dataDir); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(filepath.Join(dataDir, "tarship.db"))
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, dsn(abs))
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 同一时刻只有一个写者；限制为单连接把并发串行化，避免 "database is locked"。
	// 代价：持有 Rows 期间不能再发起新查询，所以所有读取都先读完再做下一步。
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		// 数据库文件所在目录不可写时，Ping 的底层报错是不可读的 SQLite 错误码；
		// 探针已在上面拦截了绝大多数权限场景，这里再兜底提示一次排查方向。
		return nil, fmt.Errorf("连接数据库失败: %w（若为全新部署，请检查数据目录 %s 的属主与写权限）", err, dataDir)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// probeWritable 探测数据目录可写性，把权限错误翻译成可执行的修复指引。
// 容器默认以 UID 65532 运行，宿主目录/卷属主不符时这里能给出准确提示，
// 而不是把 "unable to open database file (14)" 直接甩给用户。
func probeWritable(dataDir string) error {
	f, err := os.CreateTemp(dataDir, ".write-probe-*")
	if err != nil {
		return fmt.Errorf("数据目录 %s 不可写（当前进程 UID=%d）: %w\n"+
			"修复方式（任选其一）:\n"+
			"  1. 调整目录属主: chown -R 65532:65532 <数据目录>（容器默认以 65532 运行）\n"+
			"  2. 或在 compose 里删除 user: \"65532:65532\" 行，改回 root 运行\n"+
			"  3. 或用 --data / TARSHIP_DATA 指定一个可写目录",
			dataDir, os.Getuid(), err)
	}
	return os.Remove(f.Name())
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Ping 用于健康检查。
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("初始化迁移表失败: %w", err)
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries { // ReadDir 按文件名排序
		name := e.Name()
		prefix, _, _ := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return fmt.Errorf("迁移文件名必须以数字版本号开头: %s", name)
		}
		var exists int
		err = db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&exists)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("执行迁移 %s 失败: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
			version, fmtTime(time.Now())); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func fmtTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) { return time.Parse(timeLayout, s) }

// ---- settings ----

// GetSetting 读取设置项。
func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// EnsureSetting 返回设置项；不存在时用 gen 生成并写入。
func (s *Store) EnsureSetting(ctx context.Context, key string, gen func() (string, error)) (string, error) {
	if v, ok, err := s.GetSetting(ctx, key); err != nil || ok {
		return v, err
	}
	v, err := gen()
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key, value) VALUES(?, ?)`, key, v); err != nil {
		return "", err
	}
	got, _, err := s.GetSetting(ctx, key)
	return got, err
}

const (
	keyAdminUser = "admin_username"
	keyAdminHash = "admin_password_hash"
)

// CreateAdmin 创建唯一的管理员账号；已存在时返回 false。
func (s *Store) CreateAdmin(ctx context.Context, username, passwordHash string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO NOTHING`, keyAdminUser, username)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?)`, keyAdminHash, passwordHash); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// Admin 返回管理员账号；尚未创建时 ok 为 false。
func (s *Store) Admin(ctx context.Context) (username, passwordHash string, ok bool, err error) {
	username, ok, err = s.GetSetting(ctx, keyAdminUser)
	if err != nil || !ok {
		return "", "", false, err
	}
	passwordHash, ok, err = s.GetSetting(ctx, keyAdminHash)
	return username, passwordHash, ok, err
}

// ---- servers ----

// Server 是一台被管理的服务器。
type Server struct {
	ID           int64
	Name         string
	TokenHash    string
	Hostname     string
	AgentVersion string
	Tags         []string
	LastSeenAt   *time.Time
	CreatedAt    time.Time
}

const serverColumns = `id, name, token_hash, hostname, agent_version, tags, last_seen_at, created_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanServer(r rowScanner) (Server, error) {
	var (
		sv                    Server
		host, ver, tags, seen sql.NullString
		created               string
	)
	if err := r.Scan(&sv.ID, &sv.Name, &sv.TokenHash, &host, &ver, &tags, &seen, &created); err != nil {
		return Server{}, err
	}
	sv.Hostname = host.String
	sv.AgentVersion = ver.String
	if tags.Valid && tags.String != "" {
		// v0.1.x 旧库存的是逗号分隔文本，v0.2 起写入 JSON 数组；
		// 解析失败时回退逗号分隔，保证老库升级后列表不被单条旧数据卡死
		if err := json.Unmarshal([]byte(tags.String), &sv.Tags); err != nil {
			for _, t := range strings.Split(tags.String, ",") {
				if t = strings.TrimSpace(t); t != "" {
					sv.Tags = append(sv.Tags, t)
				}
			}
		}
	}
	if seen.Valid && seen.String != "" {
		t, err := parseTime(seen.String)
		if err != nil {
			return Server{}, err
		}
		sv.LastSeenAt = &t
	}
	t, err := parseTime(created)
	if err != nil {
		return Server{}, err
	}
	sv.CreatedAt = t
	return sv, nil
}

// CreateServer 新增服务器，tokenHash 是 token 的 SHA-256。
func (s *Store) CreateServer(ctx context.Context, name string, tags []string, tokenHash string) (Server, error) {
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return Server{}, err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO servers(name, token_hash, tags, created_at) VALUES(?, ?, ?, ?)`,
		name, tokenHash, string(tagsJSON), fmtTime(now))
	if err != nil {
		return Server{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Server{}, err
	}
	return Server{ID: id, Name: name, TokenHash: tokenHash, Tags: tags, CreatedAt: now}, nil
}

// ListServers 按创建顺序返回全部服务器。
func (s *Store) ListServers(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+serverColumns+` FROM servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Server{}
	for rows.Next() {
		sv, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sv)
	}
	return out, rows.Err()
}

// GetServer 按 ID 查询。
func (s *Store) GetServer(ctx context.Context, id int64) (Server, bool, error) {
	sv, err := scanServer(s.db.QueryRowContext(ctx, `SELECT `+serverColumns+` FROM servers WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return Server{}, false, nil
	}
	return sv, err == nil, err
}

// GetServerByTokenHash 按 token 哈希查询，用于 Agent 鉴权。
func (s *Store) GetServerByTokenHash(ctx context.Context, hash string) (Server, bool, error) {
	sv, err := scanServer(s.db.QueryRowContext(ctx, `SELECT `+serverColumns+` FROM servers WHERE token_hash = ?`, hash))
	if err == sql.ErrNoRows {
		return Server{}, false, nil
	}
	return sv, err == nil, err
}

// DeleteServer 删除服务器，不存在时返回 false。
func (s *Store) DeleteServer(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// UpdateServerToken 重置 token，不存在时返回 false。
func (s *Store) UpdateServerToken(ctx context.Context, id int64, tokenHash string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE servers SET token_hash = ? WHERE id = ?`, tokenHash, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// TouchServer 在 Agent 上线（hello）时记录主机名、版本与时间。
func (s *Store) TouchServer(ctx context.Context, id int64, hostname, agentVersion string, seen time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET hostname = ?, agent_version = ?, last_seen_at = ? WHERE id = ?`,
		hostname, agentVersion, fmtTime(seen), id)
	return err
}

// UpdateLastSeen 刷新最后在线时间。
func (s *Store) UpdateLastSeen(ctx context.Context, id int64, seen time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE servers SET last_seen_at = ? WHERE id = ?`, fmtTime(seen), id)
	return err
}
