import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    // In dev (npm run dev) the API is still served by the Go binary.
    proxy: { '/api': 'http://localhost:7373' },
  },
})
