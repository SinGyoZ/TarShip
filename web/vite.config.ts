import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// 开发时前端跑在 5173，API 与 WebSocket 代理到本机 Hub（go run ./cmd/tarship hub --data ./data）。
// 保持 changeOrigin 为默认的 false：Hub 会校验 Origin 与 Host 是否一致，改写 Host 会让校验失败。
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/ws": { target: "ws://localhost:8080", ws: true },
    },
  },
  build: { outDir: "dist", emptyOutDir: true },
});
