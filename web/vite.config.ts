import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const target = process.env.GATEWAY_URL ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  server: { proxy: { '/api': target, '/v1': target } },
})
