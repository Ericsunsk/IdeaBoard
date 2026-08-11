import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { publicSiteVitePlugin } from './src/features/public-site/publicSiteVitePlugin'

export default defineConfig({
  plugins: [vue(), tailwindcss(), publicSiteVitePlugin()],
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.unit.spec.ts', 'src/**/*.component.spec.ts', 'src/**/*.contract.spec.ts'],
    setupFiles: ['src/test/setup.ts'],
  },
  build: {
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      onwarn(warning, warn) {
        const id = warning.id || ''
        if (warning.code === 'INVALID_ANNOTATION' && id.includes('/node_modules/@vueuse/core/')) {
          return
        }
        warn(warning)
      },
      output: {
        manualChunks(id) {
          if (id.includes('/node_modules/vue-router/')) {
            return 'vue-router'
          }
          if (id.includes('/node_modules/@vue-flow/core/')) {
            return 'vue-flow'
          }
          return undefined
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api/auth': {
        target: 'http://localhost:6001',
        changeOrigin: true,
      },
      '/api': {
        target: 'http://localhost:6000',
        changeOrigin: true,
      },
      '/uploads': {
        target: 'http://localhost:6000',
        changeOrigin: true,
      },
    },
  },
})
