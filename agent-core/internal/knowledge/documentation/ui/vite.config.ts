import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
  },
  server: {
    port: 5174,
    proxy: {
      '/api': 'http://localhost:18081',
    },
  },
})
