package store

// M1 备份闭环的数据访问层：存储器、任务、运行、产物。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// nowRFC3339 返回当前 UTC 时间的 RFC3339Nano 文本，与迁移中 TEXT 时间列一致。
func nowRFC3339() string { return fmtTime(time.Now().UTC()) }

// ---- storages ----

// Storage 是一个备份产物存储器：local（Agent 同机目录）或 s3（兼容端点）。
type Storage struct {
	ID              int64
	Name            string
	Type            string // "local" | "s3"
	RootDir         string // local
	Endpoint        string // s3
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Prefix          string // s3 对象键前缀，默认 backups
	CreatedAt       time.Time
}

const storageColumns = `id, name, type, root_dir, endpoint, region, bucket, access_key_id, secret_access_key, prefix, created_at`

func scanStorage(r rowScanner) (Storage, error) {
	var (
		st                     Storage
		root, ep, region, bkt  sql.NullString
		ak, sk, prefix, created sql.NullString
	)
	if err := r.Scan(&st.ID, &st.Name, &st.Type, &root, &ep, &region, &bkt, &ak, &sk, &prefix, &created); err != nil {
		return Storage{}, err
	}
	st.RootDir, st.Endpoint, st.Region, st.Bucket = root.String, ep.String, region.String, bkt.String
	st.AccessKeyID, st.SecretAccessKey, st.Prefix = ak.String, sk.String, prefix.String
	if st.Prefix == "" {
		st.Prefix = "backups"
	}
	t, err := parseTime(created.String)
	if err != nil {
		return Storage{}, err
	}
	st.CreatedAt = t
	return st, nil
}

var ErrNotFound = errors.New("record not found")

// ValidateStorage 校验存储器字段并补默认值。
func ValidateStorage(st *Storage) error {
	st.Name = strings.TrimSpace(st.Name)
	if st.Name == "" || len([]rune(st.Name)) > 64 {
		return errors.New("存储器名称需为 1-64 个字符")
	}
	switch st.Type {
	case "local":
		if !strings.HasPrefix(st.RootDir, "/") {
			return errors.New("local 存储器需要以 / 开头的绝对路径（Agent 视角）")
		}
		st.Prefix = "backups"
		return nil
	case "s3":
		if st.Endpoint == "" || st.Bucket == "" || st.AccessKeyID == "" || st.SecretAccessKey == "" {
			return errors.New("s3 存储器需要 endpoint、bucket 与访问凭据")
		}
		if st.Prefix == "" {
			st.Prefix = "backups"
		}
		return nil
	default:
		return errors.New("存储器类型仅支持 local / s3")
	}
}

func (s *Store) CreateStorage(ctx context.Context, st Storage) (Storage, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO storages
		(name, type, root_dir, endpoint, region, bucket, access_key_id, secret_access_key, prefix, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.Name, st.Type, st.RootDir, st.Endpoint, st.Region, st.Bucket,
		st.AccessKeyID, st.SecretAccessKey, st.Prefix, nowRFC3339())
	if err != nil {
		return Storage{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetStorage(ctx, id)
}

func (s *Store) GetStorage(ctx context.Context, id int64) (Storage, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+storageColumns+` FROM storages WHERE id = ?`, id)
	st, err := scanStorage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Storage{}, ErrNotFound
	}
	return st, err
}

func (s *Store) ListStorages(ctx context.Context) ([]Storage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+storageColumns+` FROM storages ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Storage
	for rows.Next() {
		st, err := scanStorage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// UpdateStorage 全量更新存储器（不改创建时间）。
func (s *Store) UpdateStorage(ctx context.Context, id int64, st Storage) (Storage, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE storages SET
		name=?, type=?, root_dir=?, endpoint=?, region=?, bucket=?, access_key_id=?, secret_access_key=?, prefix=?
		WHERE id=?`,
		st.Name, st.Type, st.RootDir, st.Endpoint, st.Region, st.Bucket,
		st.AccessKeyID, st.SecretAccessKey, st.Prefix, id)
	if err != nil {
		return Storage{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Storage{}, ErrNotFound
	}
	return s.GetStorage(ctx, id)
}

// DeleteStorage 删除存储器；仍被任务引用时拒绝（外键 RESTRICT 之外的显式检查给出友好错误）。
func (s *Store) DeleteStorage(ctx context.Context, id int64) (bool, error) {
	var used int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE storage_id = ?`, id).Scan(&used); err != nil {
		return false, err
	}
	if used > 0 {
		return false, fmt.Errorf("仍有 %d 个任务使用该存储器，先删除或改绑任务", used)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM storages WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---- tasks ----

// Task 是一个备份任务定义。
type Task struct {
	ID            int64
	Name          string
	ServerID      int64
	StorageID     int64
	Paths         []string
	Excludes      []string
	CronExpr      string
	RetentionDays int
	Enabled       bool
	CreatedAt     time.Time
}

func marshalStrings(list []string) (string, error) {
	if list == nil {
		list = []string{}
	}
	b, err := json.Marshal(list)
	return string(b), err
}

func unmarshalStrings(s sql.NullString) ([]string, error) {
	out := []string{}
	if !s.Valid || s.String == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(s.String), &out); err != nil {
		// 兼容历史逗号分隔格式
		for _, p := range strings.Split(s.String, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

func scanTask(r rowScanner) (Task, error) {
	var (
		tk                 Task
		paths, excludes, cron, created sql.NullString
		enabled            int
	)
	if err := r.Scan(&tk.ID, &tk.Name, &tk.ServerID, &tk.StorageID, &paths, &excludes, &cron, &tk.RetentionDays, &enabled, &created); err != nil {
		return Task{}, err
	}
	var err error
	if tk.Paths, err = unmarshalStrings(paths); err != nil {
		return Task{}, err
	}
	if tk.Excludes, err = unmarshalStrings(excludes); err != nil {
		return Task{}, err
	}
	tk.CronExpr = cron.String
	tk.Enabled = enabled != 0
	if tk.CreatedAt, err = parseTime(created.String); err != nil {
		return Task{}, err
	}
	return tk, nil
}

const taskColumns = `id, name, server_id, storage_id, paths, excludes, cron_expr, retention_days, enabled, created_at`

// ValidateTask 校验任务定义。
func ValidateTask(tk *Task) error {
	tk.Name = strings.TrimSpace(tk.Name)
	if tk.Name == "" || len([]rune(tk.Name)) > 64 {
		return errors.New("任务名称需为 1-64 个字符")
	}
	if tk.ServerID <= 0 {
		return errors.New("必须选择目标服务器")
	}
	if tk.StorageID <= 0 {
		return errors.New("必须选择存储器")
	}
	if len(tk.Paths) == 0 {
		return errors.New("至少配置一个要备份的目录")
	}
	for _, p := range tk.Paths {
		if !strings.HasPrefix(p, "/") {
			return fmt.Errorf("备份路径必须以 / 开头（Agent 视角绝对路径）: %s", p)
		}
	}
	if len(tk.Excludes) > 50 {
		return errors.New("排除规则最多 50 条")
	}
	if tk.RetentionDays < 0 || tk.RetentionDays > 3650 {
		return errors.New("保留天数需在 0-3650 之间")
	}
	return nil
}

func (s *Store) CreateTask(ctx context.Context, tk Task) (Task, error) {
	paths, err := marshalStrings(tk.Paths)
	if err != nil {
		return Task{}, err
	}
	excludes, err := marshalStrings(tk.Excludes)
	if err != nil {
		return Task{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO tasks
		(name, server_id, storage_id, paths, excludes, cron_expr, retention_days, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tk.Name, tk.ServerID, tk.StorageID, paths, excludes, tk.CronExpr, tk.RetentionDays, b2i(tk.Enabled), nowRFC3339())
	if err != nil {
		return Task{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetTask(ctx, id)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) GetTask(ctx context.Context, id int64) (Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	tk, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return tk, err
}

func (s *Store) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		tk, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tk)
	}
	return out, rows.Err()
}

func (s *Store) UpdateTask(ctx context.Context, id int64, tk Task) (Task, error) {
	paths, err := marshalStrings(tk.Paths)
	if err != nil {
		return Task{}, err
	}
	excludes, err := marshalStrings(tk.Excludes)
	if err != nil {
		return Task{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE tasks SET
		name=?, server_id=?, storage_id=?, paths=?, excludes=?, cron_expr=?, retention_days=?, enabled=? WHERE id=?`,
		tk.Name, tk.ServerID, tk.StorageID, paths, excludes, tk.CronExpr, tk.RetentionDays, b2i(tk.Enabled), id)
	if err != nil {
		return Task{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Task{}, ErrNotFound
	}
	return s.GetTask(ctx, id)
}

func (s *Store) DeleteTask(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---- runs ----

// Run 是一次备份执行的记录。
type Run struct {
	ID         int64
	TaskID     int64
	ServerID   int64
	StorageID  int64
	Status     string // running / success / failed
	SizeBytes  int64
	Sha256     string
	ObjectKey  string
	ErrorMsg   string
	StartedAt  time.Time
	FinishedAt *time.Time
}

const runColumns = `id, task_id, server_id, storage_id, status, size_bytes, sha256, object_key, error_msg, started_at, finished_at`

func scanRun(r rowScanner) (Run, error) {
	var (
		rn                  Run
		size                sql.NullInt64
		sha, key, msg       sql.NullString
		started, finished  sql.NullString
	)
	if err := r.Scan(&rn.ID, &rn.TaskID, &rn.ServerID, &rn.StorageID, &rn.Status, &size, &sha, &key, &msg, &started, &finished); err != nil {
		return Run{}, err
	}
	rn.SizeBytes, rn.Sha256, rn.ObjectKey, rn.ErrorMsg = size.Int64, sha.String, key.String, msg.String
	var err error
	if rn.StartedAt, err = parseTime(started.String); err != nil {
		return Run{}, err
	}
	if finished.Valid && finished.String != "" {
		t, err := parseTime(finished.String)
		if err != nil {
			return Run{}, err
		}
		rn.FinishedAt = &t
	}
	return rn, nil
}

// CreateRun 登记一次运行；同一任务已有 running 时拒绝，防止并发重复备份。
func (s *Store) CreateRun(ctx context.Context, taskID, serverID, storageID int64) (Run, error) {
	var running int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE task_id = ? AND status = 'running'`, taskID).Scan(&running); err != nil {
		return Run{}, err
	}
	if running > 0 {
		return Run{}, errors.New("该任务已有正在进行的运行，请等待完成后再试")
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO runs
		(task_id, server_id, storage_id, status, started_at) VALUES (?, ?, ?, 'running', ?)`,
		taskID, serverID, storageID, nowRFC3339())
	if err != nil {
		return Run{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetRun(ctx, id)
}

func (s *Store) GetRun(ctx context.Context, id int64) (Run, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM runs WHERE id = ?`, id)
	rn, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return rn, err
}

func (s *Store) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+runColumns+` FROM runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		rn, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rn)
	}
	return out, rows.Err()
}

// FinishRunSuccess 结束运行并登记产物；仅 running 可流转 success，防并发重复结算。
func (s *Store) FinishRunSuccess(ctx context.Context, id int64, objectKey string, size int64, sha string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	finished := nowRFC3339()
	res, err := tx.ExecContext(ctx,
		`UPDATE runs SET status='success', size_bytes=?, sha256=?, object_key=?, finished_at=?
			 WHERE id=? AND status='running'`, size, sha, objectKey, finished, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("运行已结束，忽略重复的成功上报")
	}
	var taskID, serverID, storageID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT task_id, server_id, storage_id FROM runs WHERE id=?`, id).
		Scan(&taskID, &serverID, &storageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO backups
		(run_id, task_id, server_id, storage_id, object_key, size_bytes, sha256, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, taskID, serverID, storageID, objectKey, size, sha, finished); err != nil {
		return err
	}
	return tx.Commit()
}

// FinishRunFailed 结束运行并记录失败原因。
func (s *Store) FinishRunFailed(ctx context.Context, id int64, msg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status='failed', error_msg=?, finished_at=? WHERE id=? AND status='running'`,
		msg, nowRFC3339(), id)
	return err
}

// FailServerRuns 把某台服务器全部 running 的运行标记失败（Agent 断线时调用）。
func (s *Store) FailServerRuns(ctx context.Context, serverID int64, reason string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status='failed', error_msg=?, finished_at=? WHERE server_id=? AND status='running'`,
		reason, nowRFC3339(), serverID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// FailAllRunningRuns 在 Hub 启动时清场：上一次进程退出遗留的 running 一律失败。
func (s *Store) FailAllRunningRuns(ctx context.Context, reason string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status='failed', error_msg=?, finished_at=? WHERE status='running'`, reason, nowRFC3339())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- backups ----

// Backup 是一个备份产物的登记。
type Backup struct {
	ID         int64
	RunID      int64
	TaskID     int64
	ServerID   int64
	StorageID  int64
	ObjectKey  string
	SizeBytes  int64
	Sha256     string
	CreatedAt  time.Time
}

func (s *Store) ListBackups(ctx context.Context, limit int) ([]Backup, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, task_id, server_id, storage_id, object_key, size_bytes, sha256, created_at
		 FROM backups ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Backup
	for rows.Next() {
		var b Backup
		var created string
		if err := rows.Scan(&b.ID, &b.RunID, &b.TaskID, &b.ServerID, &b.StorageID, &b.ObjectKey, &b.SizeBytes, &b.Sha256, &created); err != nil {
			return nil, err
		}
		if b.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
