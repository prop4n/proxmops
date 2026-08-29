import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

const root = import.meta.dirname

// The daemon embeds the built assets from dist/ and serves them at /, so the
// dev server proxies /api to the running Go daemon on :8080.
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(root, './src') },
  },
  // Keep emptyOutDir false so dist/.gitkeep survives builds; that placeholder
  // keeps go:embed compiling on a fresh checkout with no UI build yet.
  build: { outDir: 'dist', emptyOutDir: false },
  server: {
    proxy: { '/api': 'http://localhost:8080' },
  },
})
