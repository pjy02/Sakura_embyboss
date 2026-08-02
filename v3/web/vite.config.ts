import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/health': 'http://127.0.0.1:8080',
      '/openapi.yaml': 'http://127.0.0.1:8080',
    },
  },
  test: { environment: 'jsdom', include: ['src/**/*.test.ts'] },
})
