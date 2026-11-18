// Package db 封装 SQLite 打开与版本化迁移。
// 使用 modernc.org/sqlite（纯 Go 实现，无 CGO），配合 CGO_ENABLED=0 静态编译。
package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite" // 注册 database/sql 驱动名 "sqlite"
)

// Open 打开（或创建）SQLite 数据库并执行迁移。
// 每个连接共用一套 pragma；WAL 模式保证 Hub 单写多读场景下读写互不阻塞。
func Open(dataDir string) (*sql.DB, error) {
	dbPath := filepath.Join(dataDir, "tarship.db")
	// DSN 中的 pragma 经查询参数下发（modernc 驱动约定：每个 _pragma 一条）
	dsn := "file:" + url.PathEscape(dbPath) +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	// SQLite 并发写依赖 busy_timeout 重试，连接池过大会放大锁冲突，收小并禁止自动建多余连接
	d.SetMaxOpenConns(4)
	d.SetMaxIdleConns(2)
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, fmt.Errorf("连接数据库: %w", err)
	}
	if err := migrate(d); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// migrate 逐版本执行迁移；schema_migrations 表记录已应用版本。
// 后续里程碑新增表时只追加 migrations 条目，历史版本不做修改。
func migrate(d *sql.DB) error {
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL
	)`); err != nil {
		return fmt.Errorf("创建迁移记录表: %w", err)
	}
	var current int
	if err := d.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("读取迁移版本: %w", err)
	}
	for i, m := range migrations {
		v := i + 1
		if v <= current {
			continue
		}
		if _, err := d.Exec(m.sql); err != nil {
			return fmt.Errorf("执行迁移 V%d: %w", v, err)
		}
		if _, err := d.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, datetime('now'))`, v); err != nil {
			return fmt.Errorf("记录迁移 V%d: %w", v, err)
		}
	}
	return nil
}
