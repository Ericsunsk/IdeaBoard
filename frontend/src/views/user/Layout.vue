<template>
  <div class="user-layout">
    <!-- ========== 顶部导航 ========== -->
    <header v-if="!['/user/home', '/user/boards', '/user/picture', '/user', '/user/center'].includes(route.path)" class="top-header">
      <n-menu
        v-model:value="activeKey"
        mode="horizontal"
        :options="menuOptions"
        :root-indent="20"
        @update:value="handleMenuSelect"
      />
    </header>

    <!-- ========== 页面内容 ========== -->
    <div class="layout-content">
      <router-view :key="route.path === '/user/picture' ? route.path : route.fullPath" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NMenu } from 'naive-ui'
import type { MenuOption } from 'naive-ui'

const router = useRouter()
const route = useRoute()

const activeKey = ref<string>('free')

const menuOptions: MenuOption[] = [
  {
    label: '首页',
    key: 'home',
  },
  {
    label: '图・文生图',
    key: 'free',
  },
  {
    label: '服装实验室',
    key: 'fashion-lab',
    children: [
      { label: '局部改款', key: 'partial-edit' },
      { label: '定向换色', key: 'directed-recolor' },
    ]
  },
  {
    label: '图案工作室',
    key: 'pattern-studio',
    children: [
      { label: '图案生成', key: 'pattern-gen' },
      { label: '四方连续', key: 'seamless-tile' },
    ]
  },
  {
    label: 'AI 工具箱',
    key: 'ai-toolbox',
    children: [
      { label: '高清放大', key: 'hd-upscale' },
      { label: '画布延伸', key: 'canvas-extend' },
    ]
  },
  {
    label: '我的',
    key: 'mine',
    children: [
      { label: '我的作品', key: 'my-works' },
    ]
  },
]

// 菜单 key → 路由路径的映射
const keyToPath: Record<string, string> = {
  home: '/user/home',
  free: '/user/boards',
  'my-works': '/user/center',
  'partial-edit': '/user/picture',
  'directed-recolor': '/user/picture',
  'pattern-gen': '/user/picture',
  'seamless-tile': '/user/picture',
  'hd-upscale': '/user/picture',
  'canvas-extend': '/user/picture',
}

function handleMenuSelect(key: string) {
  const path = keyToPath[key] || `/user/${key}`
  router.push(path)
}

// 路由路径 → 菜单 key 的映射
const pathToKey: Record<string, string> = {
  '/user/boards': 'free',
  '/user/picture': 'free',
  '/user/home': 'home',
  '/user/center': 'my-works',
}

watch(
  () => route.path,
  (path) => {
    if (pathToKey[path]) {
      activeKey.value = pathToKey[path]
    } else {
      // /user/xxx -> xxx
      const match = path.match(/^\/user\/([\w-]+)/)
      if (match) activeKey.value = match[1]
    }
  },
  { immediate: true }
)
</script>

<style scoped>
.user-layout {
  width: 100%;
  height: 100vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.top-header {
  display: flex;
  align-items: center;
  padding: 0 12px;
  background: #18181b;
  border-bottom: 1px solid #27272a;
  flex-shrink: 0;
  position: relative;
  z-index: 50;
  transition: background-color 0.3s, border-color 0.3s, transform 0.3s ease;
}

.layout-content {
  flex: 1;
  min-height: 0;
  background: transparent;
  overflow: hidden;
}

/* 默认深色模式菜单样式 */
:deep(.n-menu.n-menu--horizontal) {
  background: transparent;
  --n-item-text-color: #8a8a8f;
  --n-item-text-color-hover: #e4e4e7;
  --n-item-text-color-active: #a3e635;
  --n-item-text-color-active-hover: #bef264;
  --n-item-text-color-child-active: #a3e635;
  --n-item-text-color-child-active-hover: #bef264;
  --n-item-color-hover: rgba(163, 230, 53, 0.06);
  --n-item-color-active: rgba(163, 230, 53, 0.08);
  --n-item-color-active-hover: rgba(163, 230, 53, 0.1);
  --n-item-color-active-collapsed: rgba(163, 230, 53, 0.08);
  --n-arrow-color: #52525b;
  --n-arrow-color-hover: #a3e635;
  --n-arrow-color-active: #a3e635;
  --n-arrow-color-active-hover: #bef264;
  --n-arrow-color-child-active: #a3e635;
  --n-arrow-color-child-active-hover: #bef264;
  --n-item-height: 46px;
  --n-font-size: 14px;
  --n-border-radius: 6px;
}

:deep(.n-menu.n-menu--horizontal .n-menu-item-content) {
  padding: 0 18px !important;
}

:deep(.n-submenu.n-submenu--horizontal > .n-menu-item-content) {
  padding: 0 18px !important;
}
</style>
