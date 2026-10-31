import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  build: {
    chunkSizeWarningLimit: 800,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/gsap')) {
            return 'vendor-gsap'
          }
          if (id.includes('node_modules/lucide-svelte')) {
            return 'vendor-icons'
          }
          if (id.includes('node_modules/marked')) {
            return 'vendor-marked'
          }
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:20130',
      '/v1': 'http://localhost:20130',
      '/usage': 'http://localhost:20130',
      '/translator': 'http://localhost:20130',
      '/debug': 'http://localhost:20130',
      '/admin': 'http://localhost:20130',
    },
  },
})
