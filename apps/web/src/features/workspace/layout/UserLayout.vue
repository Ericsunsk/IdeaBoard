<template>
  <div class="user-layout">
    <div v-if="isImpersonating" class="impersonation-banner">
      <div class="impersonation-copy">
        <span class="impersonation-icon-wrap">
          <n-icon size="14">
            <ShieldHalfOutline />
          </n-icon>
        </span>
        <span class="impersonation-title">伪装预览中</span>
      </div>
      <n-button
        size="small"
        circle
        quaternary
        class="impersonation-exit"
        :loading="stopImpersonatingLoading"
        @click="handleStopImpersonating"
      >
        <template #icon>
          <n-icon>
            <CloseOutline />
          </n-icon>
        </template>
      </n-button>
    </div>

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
import type { MenuOption } from 'naive-ui'
import { CloseOutline, ShieldHalfOutline } from '@vicons/ionicons5'
import { useUserStore } from '../../../app/stores/user'
import { authClient, extractBetterAuthErrorMessage } from '../../../shared/auth/client'

const router = useRouter()
const route = useRoute()
const message = useMessage()
const userStore = useUserStore()

const activeKey = ref<string>('free')
const stopImpersonatingLoading = ref(false)
const authSession = authClient.useSession()
const isImpersonating = computed(() => Boolean((authSession.value?.data as any)?.session?.impersonatedBy))

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
  'hd-upscale': '/user/picture',
  'canvas-extend': '/user/picture',
}

function handleMenuSelect(key: string) {
  const path = keyToPath[key] || `/user/${key}`
  router.push(path)
}

async function handleStopImpersonating() {
  stopImpersonatingLoading.value = true
  try {
    const { error } = await authClient.admin.stopImpersonating()
    if (error) throw error
    await userStore.fetchUserInfo(true)
    await authSession.value?.refetch?.()
    message.success('已退出伪装身份')
    router.push('/admin/users')
  } catch (err: any) {
    message.error(extractBetterAuthErrorMessage(err, '退出伪装失败，请重新登录管理员账号'))
  } finally {
    stopImpersonatingLoading.value = false
  }
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
  background: rgba(250, 251, 255, 0.92);
  border-bottom: 1px solid #d9dfec;
  backdrop-filter: blur(10px);
  flex-shrink: 0;
  position: relative;
  z-index: 50;
  transition: background-color 0.3s, border-color 0.3s, transform 0.3s ease;
}

.impersonation-banner {
  min-height: 40px;
  padding: 6px 10px;
  background: rgba(246, 247, 255, 0.92);
  border: 1px solid rgba(122, 107, 213, 0.24);
  border-radius: 14px;
  color: #2f3750;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  position: fixed;
  top: 10px;
  left: 50%;
  transform: translateX(-50%);
  width: max-content;
  max-width: calc(100vw - 20px);
  box-shadow: none;
  backdrop-filter: blur(10px);
  z-index: 1200;
}

.impersonation-copy {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 9px;
}

.impersonation-icon-wrap {
  width: 22px;
  height: 22px;
  border-radius: 7px;
  background: rgba(122, 107, 213, 0.15);
  color: #5d4ec3;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.impersonation-title {
  font-size: 12px;
  font-weight: 600;
  color: #2f3750;
  white-space: nowrap;
}

.impersonation-exit {
  flex-shrink: 0;
  width: 26px;
  height: 26px;
  min-width: 26px;
  min-height: 26px;
  border-radius: 999px;
  --n-border: 1px solid rgba(122, 107, 213, 0.34);
  --n-border-hover: 1px solid rgba(122, 107, 213, 0.48);
  --n-border-pressed: 1px solid rgba(122, 107, 213, 0.56);
  --n-color: rgba(122, 107, 213, 0.04);
  --n-color-hover: rgba(122, 107, 213, 0.1);
  --n-color-pressed: rgba(122, 107, 213, 0.14);
  --n-padding: 0;
  --n-icon-size: 14px;
  --n-ripple-color: transparent;
}

:deep(.impersonation-exit.n-button) {
  color: #5d4ec3 !important;
  --n-text-color: #5d4ec3 !important;
  --n-text-color-hover: #5244b4 !important;
  --n-text-color-pressed: #4a3ca7 !important;
}

:deep(.impersonation-exit .n-button__content),
:deep(.impersonation-exit .n-button__icon),
:deep(.impersonation-exit .n-icon) {
  color: inherit !important;
}

@media (max-width: 720px) {
  .impersonation-banner {
    top: 8px;
    max-width: calc(100vw - 12px);
    border-radius: 12px;
    padding: 5px 8px;
  }
  .impersonation-exit {
    width: 24px;
    height: 24px;
    min-width: 24px;
    min-height: 24px;
    --n-icon-size: 13px;
  }
}

.layout-content {
  flex: 1;
  min-height: 0;
  background: transparent;
  overflow: hidden;
}

:deep(.n-menu.n-menu--horizontal) {
  background: transparent;
  --n-item-text-color: #667086;
  --n-item-text-color-hover: #2e364a;
  --n-item-text-color-active: #6d5fd0;
  --n-item-text-color-active-hover: #6d5fd0;
  --n-item-text-color-child-active: #6d5fd0;
  --n-item-text-color-child-active-hover: #6d5fd0;
  --n-item-color-hover: rgba(122, 107, 213, 0.08);
  --n-item-color-active: rgba(122, 107, 213, 0.12);
  --n-item-color-active-hover: rgba(122, 107, 213, 0.14);
  --n-item-color-active-collapsed: rgba(122, 107, 213, 0.12);
  --n-arrow-color: #8a92a8;
  --n-arrow-color-hover: #6d5fd0;
  --n-arrow-color-active: #6d5fd0;
  --n-arrow-color-active-hover: #6d5fd0;
  --n-arrow-color-child-active: #6d5fd0;
  --n-arrow-color-child-active-hover: #6d5fd0;
  --n-item-height: 46px;
  --n-font-size: 14px;
  --n-border-radius: 10px;
}

:deep(.n-menu.n-menu--horizontal .n-menu-item-content) {
  padding: 0 18px !important;
}

:deep(.n-submenu.n-submenu--horizontal > .n-menu-item-content) {
  padding: 0 18px !important;
}
</style>
