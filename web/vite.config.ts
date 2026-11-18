import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 开发模式下前端跑 5173、后端跑 8080；/api 与 /ws 代理到 Hub
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': { target: 'ws://localhost:8080', ws: true },
    },
  },
  build: {
    sourcemap: false,
    chunkSizeWarningLimit: 1024,
  },
});
