import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: '/otweb/app/',
  build: {
    outDir: 'dist',
  },
  server: {
    proxy: {
      '/otweb/api': 'http://localhost:8080',
    },
  },
})
