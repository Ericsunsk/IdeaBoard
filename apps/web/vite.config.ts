import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    tailwindcss(),
    vue(),
    AutoImport({
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
