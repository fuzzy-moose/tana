import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  test: { setupFiles: ['./src/testSetup.ts'] },
  server: {
    proxy: {
      // Preserve the browser's Host and Origin for the API's CSRF protection.
      '/api': {
        target: process.env.TANA_API_TARGET ?? 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },
})
