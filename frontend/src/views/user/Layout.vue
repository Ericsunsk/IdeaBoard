<template>
  <div class="user-layout">
    <!-- ========== 顶部导航 ========== -->
    <header v-if="!['/user/home', '/user/picture', '/user', '/user/center'].includes(route.path)" class="top-header" :class="{ 'transparent-header': isHome, 'header-hidden': !showHeader }">
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
      <router-view :key="route.fullPath" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NMenu } from 'naive-ui'
import type { MenuOption } from 'naive-ui'

const router = useRouter()
const route = useRoute()

const isHome = computed(() => 
  route.path === '/user/home' || 
  route.path === '/user' || 
  route.path === '/user/picture'
)
const showHeader = ref(true)
let lastScrollTop = 0

function handleScroll(e: Event) {
  if (!isHome.value) {
    showHeader.value = true
    return
  }
  
  const target = e.target as HTMLElement
  if (target && target.classList && target.classList.contains('home-blank')) {
    const st = target.scrollTop
    if (st > lastScrollTop && st > 50) {
      // scroll down
      showHeader.value = false
    } else if (st < lastScrollTop) {
      // scroll up
      showHeader.value = true
    }
    lastScrollTop = st
  }
}

onMounted(() => {
  window.addEventListener('scroll', handleScroll, true)
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll, true)
})

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
      { label: '线稿生成', key: 'sketch-gen' },
      { label: '线稿成款', key: 'sketch-to-design' },
      { label: '局部改款', key: 'partial-edit' },
      { label: '款式创新', key: 'style-innovate' },
      { label: '风格融合', key: 'style-fusion' },
      { label: '系列配色', key: 'series-color' },
      { label: '定向换色', key: 'directed-recolor' },
      { label: '面料上身', key: 'fabric-on-body' },
      { label: '图案上身', key: 'pattern-on-body' },
    ]
  },
  {
    label: '摄影商拍',
    key: 'photo-shoot',
    children: [
      { label: '模特多角度', key: 'multi-angle' },
      { label: '虚拟 3D', key: 'virtual-3d' },
      { label: '人台变模特', key: 'mannequin-to-model' },
      { label: '定向换脸', key: 'face-swap' },
      { label: '换背景', key: 'bg-swap' },
      { label: '款式上身', key: 'garment-on-model' },
    ]
  },
  {
    label: '图案工作室',
    key: 'pattern-studio',
    children: [
      { label: '图案生成', key: 'pattern-gen' },
      { label: '画风复刻', key: 'style-clone' },
      { label: '四方连续', key: 'seamless-tile' },
      { label: '图案融合', key: 'pattern-fusion' },
      { label: '工艺融合', key: 'craft-fusion' },
      { label: '图案涂改', key: 'pattern-inpaint' },
      { label: '图案变清晰', key: 'pattern-upscale' },
    ]
  },
  {
    label: 'AI 工具箱',
    key: 'ai-toolbox',
    children: [
      { label: '转矢量图', key: 'to-vector' },
      { label: '抠图褪底', key: 'remove-bg' },
      { label: '高清放大', key: 'hd-upscale' },
      { label: '模糊变清晰', key: 'deblur' },
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
  free: '/user/picture',
  'my-works': '/user/center',
  'sketch-gen': '/user/line-draft',
}

function handleMenuSelect(key: string) {
  const path = keyToPath[key] || `/user/${key}`
  router.push(path)
}

// 路由路径 → 菜单 key 的映射
const pathToKey: Record<string, string> = {
  '/user/picture': 'free',
  '/user/home': 'home',
  '/user/center': 'my-works',
  '/user/line-draft': 'sketch-gen',
}

watch(
  () => route.path,
  (path) => {
    if (pathToKey[path]) {
      activeKey.value = pathToKey[path]
    } else {
      // /user/sketch-to-design → sketch-to-design
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

.top-header.header-hidden {
  transform: translateY(-100%);
}

.top-header.transparent-header {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  background: transparent;
  border-bottom: 1px solid transparent;
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

/* 仅在首页透明背景时强制文字为黑色 */
.transparent-header :deep(.n-menu.n-menu--horizontal) {
  --n-item-text-color: #1d1d1f !important;
  --n-item-text-color-hover: #000000 !important;
  --n-item-text-color-active: #000000 !important;
  --n-item-text-color-active-hover: #000000 !important;
  --n-item-text-color-child-active: #000000 !important;
  --n-item-text-color-child-active-hover: #000000 !important;
  --n-arrow-color: #1d1d1f !important;
  --n-arrow-color-hover: #000000 !important;
  --n-arrow-color-active: #000000 !important;
}

.transparent-header :deep(.n-menu-item-content-header),
.transparent-header :deep(.n-menu-item-content-header a),
.transparent-header :deep(.n-submenu-children),
.transparent-header :deep(.n-menu-item-content) {
  color: #1d1d1f !important;
  font-weight: 600 !important;
}

:deep(.n-menu.n-menu--horizontal .n-menu-item-content) {
  padding: 0 18px !important;
}

:deep(.n-submenu.n-submenu--horizontal > .n-menu-item-content) {
  padding: 0 18px !important;
}
</style>
