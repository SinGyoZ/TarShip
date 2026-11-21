package agent

// 备份引擎单测：对象键格式、排除规则、tar+zstd roundtrip、local 上传的原子替换。

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/SinGyoZ/TarShip/internal/proto"
)

func testAgent() *Agent {
	return &Agent{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestBuildObjectKey(t *testing.T) {
	k := buildObjectKey("", 3, 7)
	if !strings.HasPrefix(k, "backups/3/7/") || !strings.HasSuffix(k, ".tar.zst") {
		t.Fatalf("对象键格式: %q", k)
	}
	if k := buildObjectKey("custom", 3, 7); !strings.HasPrefix(k, "custom/3/7/") {
		t.Fatalf("自定义前缀: %q", k)
	}
}

func TestMatchedExcludes(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"**/node_modules/**", "/var/www/node_modules/pkg/index.js", true},
		{"**/node_modules", "/var/www/node_modules", true},
		{"/var/www/logs/**", "/var/www/logs/2026/a.log", true},
		{"/var/www/logs/**", "/var/www/data/a.log", false},
		{"**/*.log", "/a/b/c.log", true},
		{"**/*.log", "/a/b/c.txt", false},
	}
	for _, c := range cases {
		got, err := matchedExcludes(c.path, []string{c.pattern})
		if err != nil {
			t.Fatalf("match(%q, %q): %v", c.pattern, c.path, err)
		}
		if got != c.want {
			t.Fatalf("match(%q, %q) = %v", c.pattern, c.path, got)
		}
	}
	// 非法 glob 应报错而不是静默放行
	if _, err := matchedExcludes("/a", []string{"["}); err == nil {
		t.Fatal("非法规则应报错")
	}
}

// seedBackupTree 构造测试目录树。
func seedBackupTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "<h1>hello</h1>")
	write("app/data.conf", "key=value")
	write("app/logs/old.log", "log")
	write("node_modules/pkg/index.js", "module.exports=1")
	if err := os.MkdirAll(filepath.Join(root, "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("index.html", filepath.Join(root, "link.html")); err != nil {
		t.Fatal(err)
	}
	return root
}

// readTarEntries 解 tar 并返回条目名集合（目录名带尾斜杠归一化）。
func readTarEntries(t *testing.T, rd io.Reader) map[string]string {
	t.Helper()
	tr := tar.NewReader(rd)
	out := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(hdr.Name, "/")
		if hdr.Typeflag == tar.TypeReg {
			body, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			out[name] = string(body)
		} else {
			out[name] = ""
		}
	}
}

func TestBackupTarRoundtripWithExcludes(t *testing.T) {
	root := seedBackupTree(t)
	rel := func(p string) string { return strings.TrimPrefix(filepath.Join(root, p), "/") }

	var zbuf bytes.Buffer
	zw, err := zstd.NewWriter(&zbuf)
	if err != nil {
		t.Fatal(err)
	}
	tw := newTarWriter(zw)
	excludes := []string{"**/node_modules/**", "**/logs/**", "**/link.html"}
	if err := writePathsToTar(tw, []string{root}, excludes); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	zr, err := zstd.NewReader(&zbuf)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	entries := readTarEntries(t, zr)

	if got, ok := entries[rel("index.html")]; !ok || got != "<h1>hello</h1>" {
		t.Fatalf("常规文件内容异常: %q %v", got, ok)
	}
	if got, ok := entries[rel("app/data.conf")]; !ok || got != "key=value" {
		t.Fatalf("嵌套文件异常: %q %v", got, ok)
	}
	if !containsKey(entries, rel("empty-dir")) {
		t.Fatal("空目录应保留")
	}
	for _, banned := range []string{"node_modules", "app/logs/old.log", "link.html"} {
		if containsKey(entries, rel(banned)) {
			t.Fatalf("排除规则未生效: %s", banned)
		}
	}
}

func containsKey(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}

func TestLocalUploadAtomicReplace(t *testing.T) {
	a := testAgent()
	dir := t.TempDir()
	objectKey := "backups/3/7/test.tar.zst"

	up, err := a.openLocalUpload(proto.StorageTarget{Type: "local", RootDir: dir}, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("compressed bytes")
	if _, err := up.wc.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := up.wc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := up.finish(); err != nil {
		t.Fatal(err)
	}
	// 产物落位且内容一致
	got, err := os.ReadFile(filepath.Join(dir, objectKey))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("local 上传内容异常: %v", err)
	}
	// 目录里没有 .part-* 残留
	entries, _ := os.ReadDir(filepath.Join(dir, "backups/3/7"))
	if len(entries) != 1 {
		t.Fatalf("应只有一个产物文件，实际 %d", len(entries))
	}

	// abort 路径：不留下半成品
	up2, err := a.openLocalUpload(proto.StorageTarget{Type: "local", RootDir: dir}, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	up2.abort()
	matches, _ := filepath.Glob(filepath.Join(dir, "backups/3/7/.part-*"))
	if len(matches) != 0 {
		t.Fatalf("abort 应清理临时文件: %v", matches)
	}
}

func TestLocalUploadSha256Flow(t *testing.T) {
	// openLocalUpload + MultiWriter 哈希流的组合，模拟 doBackup 的写入路径
	a := testAgent()
	dir := t.TempDir()
	up, err := a.openLocalUpload(proto.StorageTarget{Type: "local", RootDir: dir}, "backups/1/1/x.tar.zst")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	sink := io.MultiWriter(up.wc, hash)
	payload := bytes.Repeat([]byte("z"), 1<<20)
	if _, err := sink.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := up.wc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := up.finish(); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(payload)
	if hex.EncodeToString(hash.Sum(nil)) != hex.EncodeToString(want[:]) {
		t.Fatal("哈希流与写入内容不一致")
	}
}
