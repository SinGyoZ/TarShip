// agent 端存储器连通测试：local 型需要在被备份机器上验证目录可写，
// S3 型由 Hub 直接验证（凭据在 Hub 侧发起 HeadBucket），不经此路径。
package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coder/websocket"

	"github.com/SinGyoZ/TarShip/internal/proto"
)

// runStorageTest 在本机测试 local 存储目录可写性并回报。
func (a *Agent) runStorageTest(ctx context.Context, conn *websocket.Conn, msg proto.StorageTest) {
	ok, message := testDirWritable(msg.RootDir)
	if err := a.send(ctx, conn, proto.StorageTestDone{
		Type: proto.TypeStorageTestDone, TestID: msg.TestID, OK: ok, Message: message,
	}); err != nil {
		a.log.Warn("回报存储测试结果失败", "error", err)
	}
}

func testDirWritable(rootDir string) (bool, string) {
	if rootDir == "" {
		return false, "缺少存储目录"
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return false, fmt.Sprintf("创建目录失败: %v", err)
	}
	// 在目录里实际写入/删除一次，验证的不只是存在性而是可写性
	f, err := os.CreateTemp(rootDir, ".tarship-test-*")
	if err != nil {
		return false, fmt.Sprintf("目录不可写: %v", err)
	}
	path := filepath.Join(rootDir, f.Name())
	f.Close()
	if err := os.Remove(path); err != nil {
		return false, fmt.Sprintf("清理测试文件失败: %v", err)
	}
	return true, "目录可写"
}
