// vite 构建会清空 dist，这里把 .gitkeep 放回去：
// Go 的 //go:embed all:dist 要求目录非空，保证没构建前端时 go build 也能通过。
import { writeFileSync } from "node:fs";

writeFileSync(new URL("../dist/.gitkeep", import.meta.url), "");
