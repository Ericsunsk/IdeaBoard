import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { visualizer } from 'rollup-plugin-visualizer'
import { seoStaticHtmlVitePlugin } from './scripts/seoStaticHtmlVitePlugin.ts'

export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
    seoStaticHtmlVitePlugin(),
    ...(process.env.ANALYZE === 'true'
      ? [
          visualizer({
            open: false,
            filename: 'dist/stats.html',
            gzipSize: true,
            brotliSize: true,
          }),
        ]
      : []),
  ],
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.unit.spec.ts', 'src/**/*.component.spec.ts', 'src/**/*.contract.spec.ts'],
    setupFiles: ['src/test/setup.ts'],
  },
  build: {
    manifest: true,
    // heic2any is an intentionally lazy-loaded HEIC decoder. Its WASM/runtime
    // bundle is ~1.35 MB minified, but it is never part of the initial entry
    // chunks (see scripts/checkBundleSize.ts optional chunk budget).
    // Keep the warning useful for eagerly loaded code while allowing this
    // known optional chunk to stay below its explicit 1.4 MB budget.
    chunkSizeWarningLimit: 1_400,
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
