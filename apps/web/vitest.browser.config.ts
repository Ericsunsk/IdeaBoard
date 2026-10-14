import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import { playwright } from '@vitest/browser-playwright'
import type {} from '@vitest/browser-playwright'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  resolve: {
    dedupe: ['vue'],
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  plugins: [vue(), tailwindcss()],
  optimizeDeps: { include: ['qrcode.vue'] },
  test: {
    include: ['src/**/*.browser.spec.ts'],
    setupFiles: ['src/test/browserSetup.ts'],
    // Retry once on CI; local runs report the first failure.
    retry: process.env.CI ? 1 : 0,
    browser: {
      enabled: true,
      headless: true,
      screenshotFailures: false,
      commands: {
        async emulateMotion({ page }, reducedMotion: 'reduce' | 'no-preference') {
          await page.emulateMedia({ reducedMotion })
        },
      },
      provider: playwright(),
      instances: [{ browser: 'chromium' }],
    },
  },
})
