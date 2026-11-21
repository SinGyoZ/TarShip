package store

import (
	"net/url"

	// modernc.org/sqlite 是纯 Go 实现，保证 CGO_ENABLED=0 的静态单二进制。
	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

// dsn 构造带 pragma 的连接串：WAL 提升并发读，busy_timeout 避免瞬时锁冲突直接报错。
func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	return u.String()
}
