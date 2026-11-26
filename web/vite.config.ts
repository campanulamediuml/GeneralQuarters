import process from 'node:process'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 平时不跑 dev server：Go 直接从 web/dist 读盘，改完前端刷新就生效。
// 想真·热更新（改 .vue 不刷新页面）再 npm run dev：5173 起一个带 HMR 的壳，
// /api、/ws、/watch、/content 全部代理给正在跑的 Go 服务端。
// 后端换了端口就 GQ_API=http://127.0.0.1:9000 npm run dev。
const backend = process.env.GQ_API || 'http://127.0.0.1:8100'
const backendWS = backend.replace(/^http/, 'ws')

export default defineConfig({
  plugins: [vue()],
  server: {
    host: '0.0.0.0',
    // Go 服务端占 8100，dev server 用 5173，免得两边抢端口。
    port: 5173,
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
