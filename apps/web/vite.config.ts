import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    AutoImport({
      include: [/\.[tj]sx?$/, /\.vue$/, /\.vue\?vue/],
      imports: [
        'vue',
        'vue-router',
        {
          '@tanstack/vue-query': ['useMutation', 'useQuery', 'useQueryClient'],
          'naive-ui': ['useDialog', 'useMessage'],
        },
      ],
      dts: 'src/auto-imports.d.ts',
      vueTemplate: true,
    }),
    Components({
      include: [/\.vue$/, /\.vue\?vue/],
      dirs: [
        'src/features/admin/shared/components',
        'src/features/auth/components',
        'src/features/boards/components',
        'src/features/workspace/components',
      ],
      deep: true,
      dts: 'src/components.d.ts',
      resolvers: [NaiveUiResolver()],
    }),
  ],
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('/node_modules/vue-router/')) {
            return 'vue-router'
          }
          if (id.includes('/node_modules/@vue-flow/core/')) {
            return 'vue-flow'
          }
          if (
            id.includes('/node_modules/echarts/') ||
            id.includes('/node_modules/zrender/') ||
            id.includes('/node_modules/vue-echarts/') ||
            id.includes('/node_modules/vue-demi/')
          ) {
            return 'echarts'
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
