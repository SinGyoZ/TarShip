// agent 备份引擎：tar 流式打包（排除规则）→ zstd 压缩 → sha256 校验 → 直传存储器。
// S3 走流式 multipart，数据从不落地 Agent 本地磁盘。
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/coder/websocket"
	"github.com/klauspost/compress/zstd"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/SinGyoZ/TarShip/internal/proto"
)

// runProgressMinInterval 进度上报最小间隔：更频繁的更新只会占用长连接带宽。
const runProgressMinInterval = 5 * time.Second

// upload 是一次产物写入句柄：写完 Finish 落位，失败 Abort 清理半成品。
type upload struct {
	wc     io.WriteCloser
	finish func() error
	abort  func()
}

// runBackup 执行一次备份并回报结果；任何错误路径都会让 run 落到 failed。
func (a *Agent) runBackup(ctx context.Context, conn *websocket.Conn, msg proto.TaskBackup) {
	start := time.Now()
	objectKey := buildObjectKey(msg.Storage.Prefix, msg.ServerID, msg.TaskID)
	a.log.Info("开始备份", "run_id", msg.RunID, "paths", strings.Join(msg.Paths, ","), "object", objectKey)

	if err := a.doBackup(ctx, conn, msg, objectKey); err != nil {
		a.log.Error("备份失败", "run_id", msg.RunID, "error", err)
		_ = a.send(ctx, conn, proto.RunFailed{Type: proto.TypeRunFailed, RunID: msg.RunID, Message: err.Error()})
		return
	}
	a.log.Info("备份完成", "run_id", msg.RunID, "object", objectKey, "elapsed", time.Since(start).Round(time.Millisecond))
}

func (a *Agent) doBackup(ctx context.Context, conn *websocket.Conn, msg proto.TaskBackup, objectKey string) error {
	if len(msg.Paths) == 0 {
		return errors.New("任务没有配置要备份的目录")
	}
	for _, p := range msg.Paths {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("备份路径不可访问: %w", err)
		}
	}

	up, err := a.openUpload(ctx, msg.Storage, objectKey)
	if err != nil {
		return err
	}

	// 数据流：tar → zstd → (存储器 + sha256 + 进度计数)
	hash := sha256.New()
	counter := &progressCounter{agent: a, conn: conn, runID: msg.RunID}
	zw, err := zstd.NewWriter(io.MultiWriter(up.wc, hash, counter))
	if err != nil {
		up.abort()
		return fmt.Errorf("初始化压缩器失败: %w", err)
	}
	tw := newTarWriter(zw)
	writeErr := writePathsToTar(tw, msg.Paths, msg.Excludes)
	// 无论成败都要按依赖顺序关闭，让流中数据完整落位
	if e := tw.Close(); e != nil && writeErr == nil {
		writeErr = e
	}
	if e := zw.Close(); e != nil && writeErr == nil {
		writeErr = e
	}
	if e := up.wc.Close(); e != nil && writeErr == nil {
		writeErr = e
	}
	if writeErr != nil {
		up.abort()
		return fmt.Errorf("打包失败: %w", writeErr)
	}
	if err := up.finish(); err != nil {
		return err
	}
	_ = counter.report(ctx, counter.total)

	return a.send(ctx, conn, proto.RunDone{
		Type: proto.TypeRunDone, RunID: msg.RunID,
		ObjectKey: objectKey, SizeBytes: counter.total, Sha256: hex.EncodeToString(hash.Sum(nil)),
	})
}

// buildObjectKey 生成产物对象键：{prefix}/{server_id}/{task_id}/{时间戳}.tar.zst（方案 2.3）。
func buildObjectKey(prefix string, serverID, taskID int64) string {
	if prefix == "" {
		prefix = "backups"
	}
	return fmt.Sprintf("%s/%d/%d/%s.tar.zst", prefix, serverID, taskID, time.Now().UTC().Format("20060102-150405"))
}

// openUpload 按存储器类型建立写入句柄。
func (a *Agent) openUpload(ctx context.Context, st proto.StorageTarget, objectKey string) (*upload, error) {
	switch st.Type {
	case "local":
		return a.openLocalUpload(st, objectKey)
	case "s3":
		return a.openS3Upload(ctx, st, objectKey)
	default:
		return nil, fmt.Errorf("未知存储器类型: %s", st.Type)
	}
}

// openLocalUpload 写 Agent 同机目录：先写临时文件，成功后原子替换，半成品不会留在产物位置。
func (a *Agent) openLocalUpload(st proto.StorageTarget, objectKey string) (*upload, error) {
	dst := filepath.Join(st.RootDir, objectKey)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, fmt.Errorf("创建存储目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".part-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %w", err)
	}
	path := tmp.Name()
	return &upload{
		wc: tmp,
		finish: func() error {
			// 调用方已 Close 写端，这里按路径 chmod；失败先清理半成品
			if err := os.Chmod(path, 0o644); err != nil {
				os.Remove(path)
				return err
			}
			return os.Rename(path, dst)
		},
		abort: func() { os.Remove(path) },
	}, nil
}

// openS3Upload 流式 multipart 上传：tar 流直接进管道，不在 Agent 落盘。
func (a *Agent) openS3Upload(ctx context.Context, st proto.StorageTarget, objectKey string) (*upload, error) {
	if st.Endpoint == "" || st.Bucket == "" {
		return nil, errors.New("s3 存储器缺少 endpoint 或 bucket")
	}
	client, err := minio.New(st.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(st.AccessKeyID, st.SecretAccessKey, st.Region),
		Secure: isHTTPSURL(st.Endpoint),
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 S3 客户端失败: %w", err)
	}
	pr, pw := io.Pipe()
	putDone := make(chan error, 1)
	go func() {
		_, err := client.PutObject(ctx, st.Bucket, objectKey, pr, -1,
			minio.PutObjectOptions{PartSize: 64 << 20})
		putDone <- err
	}()
	return &upload{
		wc: pw,
		finish: func() error {
			if err := pw.Close(); err != nil {
				return err
			}
			// PutObject 的结果只能在读端结束后确认
			if err := <-putDone; err != nil {
				return fmt.Errorf("上传失败: %w", err)
			}
			return nil
		},
		abort: func() {
			// 关闭管道让 PutObject 尽快失败退出，半成品由服务端 multipart abort 语义处理
			_ = pw.CloseWithError(errors.New("aborted"))
			<-putDone
		},
	}, nil
}

// isHTTPSURL 判定端点是否走 TLS：MinIO 客户端以 Secure 区分 http/https；
// 未带协议前缀时默认 https（端点通常形如 s3.example.com 或 https://s3.example.com）。
func isHTTPSURL(endpoint string) bool {
	return !strings.HasPrefix(strings.ToLower(endpoint), "http://")
}

// progressCounter 统计已写入字节并节流上报进度。
type progressCounter struct {
	agent     *Agent
	conn      *websocket.Conn
	runID     int64
	total     int64
	lastSent  int64
	lastFlush time.Time
}

func (c *progressCounter) Write(p []byte) (int, error) {
	c.total += int64(len(p))
	if time.Since(c.lastFlush) >= runProgressMinInterval && c.total != c.lastSent {
		c.lastSent = c.total
		c.lastFlush = time.Now()
		// 进度丢失不影响正确性（run_done 才是权威结算），失败静默
		_ = c.agent.send(context.Background(), c.conn, proto.RunProgress{
			Type: proto.TypeRunProgress, RunID: c.runID, UploadedBytes: c.total,
		})
	}
	return len(p), nil
}

func (c *progressCounter) report(ctx context.Context, n int64) error {
	return c.agent.send(ctx, c.conn, proto.RunProgress{
		Type: proto.TypeRunProgress, RunID: c.runID, UploadedBytes: n,
	})
}

// matchedExcludes 判断绝对路径是否命中任一排除规则；pattern 须为合法 doublestar glob。
func matchedExcludes(absPath string, excludes []string) (bool, error) {
	for _, pat := range excludes {
		ok, err := doublestar.Match(pat, absPath)
		if err != nil {
			return false, fmt.Errorf("排除规则 %q 不合法: %w", pat, err)
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}
