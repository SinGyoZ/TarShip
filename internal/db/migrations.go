package db

// migration 为单条版本化迁移；只追加、不修改已发布条目。
type migration struct {
	sql string
}

// migrations 按版本号升序排列。V1 只建 M0 需要的 servers 与 settings；
// storages / tasks / runs / backups / restores 由 M1 起以新版本追加。
// DDL 与方案 v1.2 第六章保持一致（DATETIME 以 RFC3339 文本存储）。
var migrations = []migration{
	{
		// 用 IF NOT EXISTS 保证幂等：若数据目录被复用于已含 servers 表的旧库
		//（schema_migrations 尚不存在），迁移不至于崩溃循环，由部署方决定是否换新库
		sql: `
CREATE TABLE IF NOT EXISTS servers (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  token_hash    TEXT NOT NULL UNIQUE,
  hostname      TEXT,
  agent_version TEXT,
  tags          TEXT,
  last_seen_at  DATETIME,
  created_at    DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`,
	},
}
