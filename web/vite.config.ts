import process from 'node:process'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 开发时前端独立跑（vite dev server 带热更新）：页面改一行立刻生效，
// 后端还是那个 Go 进程，只靠这个代理接 /api、/ws、/watch。
// 后端换了端口就 GQ_API=http://127.0.0.1:9000 npm run dev。
const backend = process.env.GQ_API || 'http://127.0.0.1:8100'
const backendWS = backend.replace(/^http/, 'ws')

export default defineConfig({
  plugins: [vue()],
  server: {
    host: '0.0.0.0',
    // 前端自己占 8099，后端 Go 在 8100：两边各是各的进程，前端改完热更新，后端不用重启。
    port: 8099,
    strictPort: true,
    proxy: {
      '/api': backend,
      '/ws': { target: backendWS, ws: true },
      '/watch': backend,
      '/content': backend,
      '/healthz': backend,
    },
  },
})
