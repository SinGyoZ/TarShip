-- 时间列统一用 TEXT 存 RFC3339（UTC），避免驱动对 DATETIME 列做隐式类型转换。
CREATE TABLE servers (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  token_hash    TEXT NOT NULL UNIQUE,   -- Agent token 的 SHA-256，明文不落库
  hostname      TEXT,
  agent_version TEXT,
  tags          TEXT,                   -- JSON 数组
  last_seen_at  TEXT,
  created_at    TEXT NOT NULL
);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
