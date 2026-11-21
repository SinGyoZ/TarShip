// Package proto 定义 Agent 与 Hub 之间 WebSocket 长连接上的消息格式。
// 消息全部为 JSON 文本帧，便于抓包调试。
package proto

import (
	"encoding/json"
	"time"
)

const (
	// Version 是协议版本，出现不兼容变更时递增。Agent 不自动升级，所以 Hub 必须能识别版本差异。
	Version = 1

	// AgentWSPath 是 Agent 连接 Hub 的路径。
	AgentWSPath = "/ws/agent"

	// HeartbeatInterval 是 Agent 发送心跳的间隔。
	HeartbeatInterval = 30 * time.Second
	// OfflineTimeout 内没有收到对端任何消息，就判定连接失效。
	// 两端对称使用：Hub 靠它判定 Agent 离线，Agent 靠它发现 Hub 半死连接。
	OfflineTimeout = 90 * time.Second
	// HelloTimeout 是连接建立后等待首条 hello 的时限。
	HelloTimeout = 10 * time.Second
	// WriteTimeout 是单次写入的时限。
	WriteTimeout = 10 * time.Second
	// MaxMessageBytes 是单条消息的大小上限。
	MaxMessageBytes = 64 << 10
)

// WebSocket 关闭码，4000-4999 是协议留给应用自定义的区间。
const (
	CloseReplaced         = 4000 // 同一 token 有新的连接，旧连接被顶掉
	CloseProtocolMismatch = 4001 // 协议版本不兼容
	CloseBadHello         = 4002 // 首条消息不是合法的 hello
	CloseRevoked          = 4003 // 服务器被删除或 token 被重置
)

// 消息类型。
const (
	TypeHello        = "hello"
	TypeHeartbeat    = "heartbeat"
	TypeWelcome      = "welcome"
	TypeHeartbeatAck = "heartbeat_ack"

	// M1 备份闭环；追加不破坏兼容，老 Agent 对未知消息默认忽略，protocol_version 维持 1
	TypeTaskBackup      = "task_backup"       // hub → agent：执行一次备份
	TypeStorageTest     = "storage_test"      // hub → agent：测试 local 存储器可写性
	TypeRunProgress      = "run_progress"      // agent → hub：备份进度
	TypeRunDone          = "run_done"          // agent → hub：备份成功
	TypeRunFailed        = "run_failed"        // agent → hub：备份失败
	TypeStorageTestDone  = "storage_test_done" // agent → hub：存储测试结果
)

// Envelope 用于先读出 type 字段再决定如何解析整条消息。
type Envelope struct {
	Type string `json:"type"`
}

// Hello 是 Agent 连接后发送的第一条消息。
type Hello struct {
	Type            string `json:"type"`
	AgentVersion    string `json:"agent_version"`
	Hostname        string `json:"hostname"`
	ProtocolVersion int    `json:"protocol_version"`
}

// Heartbeat 由 Agent 每 HeartbeatInterval 发送一次。
type Heartbeat struct {
	Type string `json:"type"`
	TS   int64  `json:"ts"`
}

// Welcome 是 Hub 对 hello 的应答，表示注册成功。
type Welcome struct {
	Type                 string `json:"type"`
	ServerID             int64  `json:"server_id"`
	Name                 string `json:"name"`
	HubVersion           string `json:"hub_version"`
	ProtocolVersion      int    `json:"protocol_version"`
	HeartbeatIntervalSec int    `json:"heartbeat_interval_sec"`
}

// HeartbeatAck 是 Hub 对每次心跳的应答，让 Agent 能发现半死连接。
type HeartbeatAck struct {
	Type string `json:"type"`
}

// StorageTarget 是随指令下发的存储器配置（凭据经 wss 通道传输；M1 明文，M4 换 age 加密）。
type StorageTarget struct {
	Type            string `json:"type"` // local | s3
	RootDir         string `json:"root_dir,omitempty"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	Prefix          string `json:"prefix,omitempty"`
}

// TaskBackup 指示 Agent 执行一次备份。
type TaskBackup struct {
	Type     string        `json:"type"`
	RunID    int64         `json:"run_id"`
	TaskID   int64         `json:"task_id"`
	ServerID int64         `json:"server_id"`
	Paths    []string      `json:"paths"`
	Excludes []string      `json:"excludes"`
	Storage  StorageTarget `json:"storage"`
}

// StorageTest 指示 Agent 测试 local 存储器在该机器上的可写性。
type StorageTest struct {
	Type     string        `json:"type"`
	TestID   string        `json:"test_id"`
	RootDir  string        `json:"root_dir"`
	Storage  StorageTarget `json:"storage"`
}

// StorageTestDone 回报存储测试结果。
type StorageTestDone struct {
	Type    string `json:"type"`
	TestID  string `json:"test_id"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// RunProgress 上报备份进度（已上传字节；流式打包无法预知总量，用绝对值）。
type RunProgress struct {
	Type           string `json:"type"`
	RunID          int64  `json:"run_id"`
	UploadedBytes  int64  `json:"uploaded_bytes"`
}

// RunDone 上报备份成功。
type RunDone struct {
	Type      string `json:"type"`
	RunID     int64  `json:"run_id"`
	ObjectKey string `json:"object_key"`
	SizeBytes int64  `json:"size_bytes"`
	Sha256    string `json:"sha256"`
}

// RunFailed 上报备份失败。
type RunFailed struct {
	Type    string `json:"type"`
	RunID   int64  `json:"run_id"`
	Message string `json:"message"`
}

// PeekType 只解析 type 字段。
func PeekType(data []byte) (string, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return "", err
	}
	return env.Type, nil
}
