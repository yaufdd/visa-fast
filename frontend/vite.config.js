import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Forward /api/* to the Go backend running on localhost:8081 so that
      // relative URLs (used by apiFetch) work under `npm run dev` as well
      // as in production (where nginx handles the same proxying).
      '/api': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
    },
  },
})
