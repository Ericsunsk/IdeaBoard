import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
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
