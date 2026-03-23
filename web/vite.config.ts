import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      // 开发模式下代理 API 请求到 Go 后端
      '/api': 'http://localhost:8080',
    },
  },
  build: {
    outDir: '../internal/server/dist',
    emptyOutDir: true,
  },
})
