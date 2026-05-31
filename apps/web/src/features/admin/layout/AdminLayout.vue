<template>
  <n-config-provider :theme-overrides="adminThemeOverrides">
    <n-dialog-provider>
      <n-notification-provider>
        <n-message-provider>
          <router-view v-if="isLoginRoute" />
          <template v-else>
            <div class="admin-layout-shell">
              <div class="admin-shell-frame">
                <aside v-if="!isMobile" class="admin-shell-sider" :class="{ collapsed }">
                  <div class="brand-area" @click="collapsed = !collapsed">
                    <div class="brand-mark">I</div>
                    <div v-if="!collapsed" class="brand-copy">
                      <p class="brand-title">IdeaBoard</p>
                    </div>
                  </div>
                  <n-menu
                    v-model:value="activeKey"
                    :collapsed="collapsed"
                    :collapsed-width="76"
                    :collapsed-icon-size="18"
                    :options="menuOptions"
                    @update:value="handleMenuUpdate"
                  />
                </aside>

                <main class="admin-shell-main">
                  <header class="admin-shell-header">
                    <div class="header-left">
                      <n-button
                        quaternary
                        circle
                        class="shell-action-btn"
                        @click="handleToggleMenu"
                      >
                        <template #icon>
                          <span>{{ isMobile ? '☰' : collapsed ? '☰' : '⟨' }}</span>
                        </template>
                      </n-button>
                      <div class="title-wrap">
                        <h1 class="page-title">
                          {{ currentMeta.title }}
                        </h1>
                      </div>
                    </div>
                    <div class="header-right">
                      <n-dropdown :options="userOptions" @select="handleUserAction">
                        <div class="user-badge">
                          <n-avatar size="small" round class="user-avatar">
                            {{ userStore.user?.username?.charAt(0).toUpperCase() }}
                          </n-avatar>
                          <span>{{ userStore.user?.username }}</span>
                        </div>
                      </n-dropdown>
                    </div>
                  </header>

                  <section class="admin-shell-content">
                    <router-view />
                  </section>
                </main>
              </div>
            </div>

            <n-drawer
              v-model:show="mobileMenuOpen"
              :width="268"
              placement="left"
              class="admin-mobile-drawer"
            >
              <n-drawer-content title="IdeaBoard" closable>
                <n-menu
                  v-model:value="activeKey"
                  :options="menuOptions"
                  @update:value="handleMobileMenuSelect"
                />
              </n-drawer-content>
            </n-drawer>
          </template>
        </n-message-provider>
      </n-notification-provider>
    </n-dialog-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import {
  NConfigProvider,
  NMenu,
  NDropdown,
  NAvatar,
  NButton,
  NIcon,
  NDrawer,
  NDrawerContent,
  NDialogProvider,
  NNotificationProvider,
  NMessageProvider,
} from 'naive-ui'
import { ChatBubble, Cloud, DashboardSpeed, Group, Link, List } from '@iconoir/vue'
import { adminThemeOverrides } from '../shared/adminThemeOverrides'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const collapsed = ref(false)
const activeKey = ref('users')
const mobileMenuOpen = ref(false)
const isMobile = ref(false)
const isLoginRoute = computed(() => route.path === '/admin/login')

watch(
  () => route.path,
  (path) => {
    const match = path.match(/^\/admin\/([\w-]+)/)
    if (match) {
      activeKey.value = match[1]
    }
  },
  { immediate: true },
)

const renderMenuIcon = (iconComp: any) => () =>
  h(NIcon, { class: 'menu-icon' }, { default: () => h(iconComp) })

type AdminMenuItem = {
  key: string
  label: string
  title?: string
  icon: any
}

const adminMenuItems: AdminMenuItem[] = [
  {
    key: 'users',
    label: '用户管理',
    title: '用户管理',
    icon: Group,
  },
  {
    key: 'channels',
    label: '渠道商管理',
    title: '渠道商管理',
    icon: Link,
  },
  {
    key: 'statistics',
    label: '数据仪表盘',
    title: '数据仪表盘',
    icon: DashboardSpeed,
  },
  {
    key: 'task-monitor',
    label: '任务监控',
    title: '任务监控',
    icon: List,
  },
  {
    key: 'feature-prompts',
    label: '系统提示词',
    title: '系统提示词',
    icon: ChatBubble,
  },
  {
    key: 'oss-config',
    label: '系统存储',
    title: '系统存储',
    icon: Cloud,
  },
] as const

const menuOptions = adminMenuItems.map((item) => ({
  label: item.label,
  key: item.key,
  icon: renderMenuIcon(item.icon),
})) as any

const pageMetaMap = adminMenuItems.reduce<Record<string, { title: string }>>((acc, item) => {
  acc[item.key] = { title: item.title || item.label }
  return acc
}, {})

const currentMeta = computed(() => {
  return pageMetaMap[activeKey.value] || { title: '管理后台' }
})

function handleMenuUpdate(key: string) {
  void router.push(`/admin/${key}`)
}

function handleMobileMenuSelect(key: string) {
  handleMenuUpdate(key)
  mobileMenuOpen.value = false
}

function handleToggleMenu() {
  if (isMobile.value) {
    mobileMenuOpen.value = true
    return
  }
  collapsed.value = !collapsed.value
}

function syncViewport() {
  isMobile.value = window.innerWidth < 1080
  if (isMobile.value) {
    collapsed.value = true
  }
}

const userOptions = [
  { label: '个人中心', key: 'profile' },
  { type: 'divider', key: 'd1' },
  { label: '退出登录', key: 'logout' },
]

function handleUserAction(key: string) {
  if (key === 'logout') {
    userStore.logout('/admin/login')
  } else if (key === 'profile') {
    router.push('/user/center')
  }
}

function setAdminRouteClass(enabled: boolean) {
  document.body.classList.toggle('admin-route', enabled)
}

onMounted(() => {
  setAdminRouteClass(true)
  syncViewport()
  window.addEventListener('resize', syncViewport)
})

onUnmounted(() => {
  setAdminRouteClass(false)
  window.removeEventListener('resize', syncViewport)
})
</script>

<style scoped>
.admin-layout-shell {
  min-height: 100vh;
  height: 100vh;
  background: #f8f9fd;
  padding: 12px;
  overflow: hidden;
}

.admin-shell-frame {
  height: calc(100vh - 24px);
  min-height: calc(100vh - 24px);
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 12px;
}

.admin-shell-sider {
  width: 244px;
  height: 100%;
  display: flex;
  flex-direction: column;
  border: 1px solid #d9dfec;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.94);
  transition: width 0.24s ease;
  box-shadow: 0 12px 30px rgba(41, 48, 66, 0.08);
}

.admin-shell-sider.collapsed {
  width: 76px;
}

.brand-area {
  height: 68px;
  padding: 0 18px;
  display: flex;
  align-items: center;
  gap: 12px;
  cursor: pointer;
  border-bottom: 1px solid #e2e4e8;
}

.brand-area:hover {
  background: #f4f6fb;
}

.brand-mark {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  font-weight: 700;
  color: #ffffff;
  background: #7a6bd5;
  border: 1px solid #7a6bd5;
}

.brand-title {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: #293042;
  letter-spacing: 0.02em;
}

.admin-shell-main {
  min-width: 0;
  min-height: 0;
  border: 1px solid #d9dfec;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.96);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 16px 38px rgba(41, 48, 66, 0.08);
}

.admin-shell-header {
  height: 68px;
  padding: 0 22px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #e2e4e8;
}

.header-left,
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.shell-action-btn {
  font-size: 16px;
  color: #6e7487;
  border: 1px solid #d9dfec;
  background: #ffffff;
  transition:
    border-color 0.18s ease,
    background 0.18s ease,
    color 0.18s ease;
}

.shell-action-btn:hover {
  border-color: #c8d0e4;
  background: #f7f8fc;
  color: #293042;
}

.title-wrap {
  min-width: 0;
}

.page-title {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  color: #293042;
}

.user-badge {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 8px 10px;
  border-radius: 999px;
  border: 1px solid #d9dfec;
  background: rgba(255, 255, 255, 0.9);
  transition:
    border-color 0.18s ease,
    background 0.18s ease;
}

.user-badge:hover {
  background: #f7f8fc;
  border-color: #c8d0e4;
}

.user-badge span {
  color: #293042;
  font-size: 13px;
  max-width: 120px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.user-avatar {
  background: #7a6bd5;
  color: #ffffff;
  font-weight: 700;
}

.admin-shell-content {
  flex: 1;
  min-height: 0;
  padding: 18px 20px 22px;
  overflow-y: auto;
  overflow-y: overlay;
  overscroll-behavior: contain;
  scrollbar-width: thin;
  scrollbar-color: rgba(116, 126, 154, 0.4) transparent;
}

.admin-shell-content::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.admin-shell-content::-webkit-scrollbar-track {
  background: transparent;
}

.admin-shell-content::-webkit-scrollbar-thumb {
  border: 2px solid transparent;
  border-radius: 999px;
  background: rgba(116, 126, 154, 0.4);
  background-clip: padding-box;
}

.admin-shell-content::-webkit-scrollbar-thumb:hover {
  background: rgba(116, 126, 154, 0.58);
  background-clip: padding-box;
}

.admin-shell-content::-webkit-scrollbar-corner {
  background: transparent;
}

:deep(.n-menu) {
  --n-item-text-color: #6e7487;
  --n-item-text-color-active: #293042;
  --n-item-text-color-hover: #293042;
  --n-item-color-active: rgba(122, 107, 213, 0.12);
  --n-item-color-hover: rgba(122, 107, 213, 0.08);
  --n-item-icon-color: #8a90a3;
  --n-item-icon-color-active: #7a6bd5;
  --n-item-icon-color-hover: #293042;
  --n-group-header-text-color: #8a90a3;
  padding: 10px 8px;
  gap: 4px;
}

.admin-shell-sider :deep(.n-menu) {
  flex: 1;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.admin-shell-sider.collapsed :deep(.n-menu-item-content) {
  grid-template-areas: 'icon' !important;
  grid-template-columns: 1fr !important;
  justify-items: center;
  padding-left: 0 !important;
  padding-right: 0 !important;
}

.admin-shell-sider.collapsed :deep(.n-menu-item-content__icon) {
  grid-area: icon;
  justify-self: center;
  margin-right: 0 !important;
}

.admin-shell-sider.collapsed :deep(.n-menu-item-content-header),
.admin-shell-sider.collapsed :deep(.n-menu-item-content__arrow) {
  display: none;
}

:deep(.n-menu-item-content) {
  border-radius: 10px !important;
  font-size: 13px;
  height: 36px;
}

:deep(.n-menu-item-content__icon) {
  margin-right: 10px !important;
}

:deep(.n-menu-item-content--selected) {
  font-weight: 600;
}

:deep(.n-menu-item-group-title) {
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-size: 11px;
}

.menu-icon {
  font-size: 17px;
}

:deep(.admin-mobile-drawer .n-drawer-content) {
  --n-color: #ffffff;
  --n-border-color: #d9dfec;
}

:deep(.admin-mobile-drawer .n-drawer-header) {
  border-bottom: 1px solid #e2e4e8;
}

:deep(.admin-mobile-drawer .n-drawer-header__main) {
  color: #293042;
  font-weight: 700;
  letter-spacing: 0.02em;
}

:deep(.admin-mobile-drawer .n-drawer-body-content-wrapper) {
  background: #f8f9fd;
}

@media (max-width: 1079px) {
  .admin-layout-shell {
    height: auto;
    padding: 10px;
    overflow: visible;
  }

  .admin-shell-frame {
    display: block;
    height: auto;
    min-height: calc(100vh - 20px);
  }

  .admin-shell-main {
    min-height: unset;
    border-radius: 14px;
    overflow: visible;
  }

  .admin-shell-header {
    padding: 0 14px;
    height: 68px;
  }

  .admin-shell-content {
    overflow-y: visible;
    padding: 12px;
    min-height: calc(100vh - 90px);
  }
}
</style>
