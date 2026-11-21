-- M1 备份闭环：存储器、任务、运行、产物四张表。
-- 时间列沿用 TEXT 存 RFC3339Nano（UTC）；JSON 数组沿用 TEXT 列。
-- runs 保留完整历史（含失败），成功后额外在 backups 登记一条产物，M2 回收站基于 backups 软删。

CREATE TABLE storages (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  name             TEXT NOT NULL,
  type             TEXT NOT NULL CHECK (type IN ('local', 's3')),
  root_dir         TEXT,            -- local：Agent 视角的绝对路径（存储器与被备份机同机时使用）
  endpoint         TEXT,            -- s3：兼容 MinIO/R2/B2 自定义端点
  region           TEXT,
  bucket           TEXT,
  access_key_id    TEXT,
  secret_access_key TEXT,          -- M1 明文存库，M4 换 age 加密（方案第九章）
  prefix           TEXT,            -- s3：对象键前缀，默认 backups
  created_at       TEXT NOT NULL
);

CREATE TABLE tasks (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  name          TEXT NOT NULL,
  server_id     INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  storage_id    INTEGER NOT NULL REFERENCES storages(id) ON DELETE RESTRICT,
  paths         TEXT NOT NULL,      -- JSON 数组：被备份的目录（Agent 视角）
  excludes      TEXT NOT NULL DEFAULT '[]', -- JSON 数组：doublestar glob
  cron_expr     TEXT,               -- M2 调度使用，M1 仅存储
  retention_days INTEGER,           -- M2 保留策略使用，M1 仅存储
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL
);

CREATE TABLE runs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  server_id   INTEGER NOT NULL,
  storage_id  INTEGER NOT NULL,
  status      TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failed')),
  size_bytes  INTEGER,
  sha256      TEXT,
  object_key  TEXT,
  error_msg   TEXT,
  started_at  TEXT NOT NULL,
  finished_at TEXT
);

CREATE TABLE backups (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id      INTEGER NOT NULL UNIQUE REFERENCES runs(id) ON DELETE CASCADE,
  task_id     INTEGER NOT NULL,
  server_id   INTEGER NOT NULL,
  storage_id INTEGER NOT NULL,
  object_key  TEXT NOT NULL,
  size_bytes  INTEGER NOT NULL,
  sha256      TEXT NOT NULL,
  created_at  TEXT NOT NULL
);

CREATE INDEX idx_runs_task ON runs(task_id, started_at DESC);
CREATE INDEX idx_backups_server ON backups(server_id, created_at DESC);
