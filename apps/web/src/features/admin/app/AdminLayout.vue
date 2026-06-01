<template>
  <n-config-provider :theme-overrides="adminThemeOverrides">
    <n-dialog-provider>
      <n-notification-provider>
        <n-message-provider>
          <router-view v-if="isLoginRoute" />
          <template v-else>
            <div class="admin-layout-shell" :style="adminLayoutStyleVars">
              <div class="admin-shell-frame">
                <aside v-if="!isMobile" class="admin-shell-sider" :class="{ collapsed }">
                  <div class="brand-area">
                    <div class="brand-copy">
                      <p class="brand-title">{{ collapsed ? 'B' : 'Dashboard' }}</p>
                    </div>
                  </div>
                  <n-menu
                    v-model:value="activeKey"
                    class="admin-nav-menu"
                    :collapsed="collapsed"
                    :icon-size="22"
                    :collapsed-width="60"
                    :collapsed-icon-size="22"
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
                          <n-icon>
                            <MenuIcon v-if="isMobile" />
                            <SidebarExpand v-else-if="collapsed" />
                            <SidebarCollapse v-else />
                          </n-icon>
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
                          <span class="user-avatar" aria-hidden="true">
                            <img
                              v-if="userStore.user?.avatar"
                              :src="userStore.user.avatar"
                              alt=""
                            />
                            <span v-else class="user-avatar-placeholder">
                              <UserIcon class="user-avatar-icon" />
                            </span>
                          </span>
                          <span class="user-name">{{ userStore.user?.username }}</span>
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
              :style="adminLayoutStyleVars"
            >
              <n-drawer-content title="Dashboard" closable>
                <n-menu
                  v-model:value="activeKey"
                  class="admin-nav-menu"
                  :icon-size="22"
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
  NButton,
  NIcon,
  NDrawer,
  NDrawerContent,
  NDialogProvider,
  NNotificationProvider,
  NMessageProvider,
} from 'naive-ui'
import type { AdminNavigationKey } from './adminNavigation'
import { Menu as MenuIcon, SidebarCollapse, SidebarExpand, User as UserIcon } from '@iconoir/vue'
import {
  adminNavigationByKey,
  adminNavigationItems,
  ADMIN_LOGIN_PATH,
  findAdminNavigationItemByPath,
} from './adminNavigation'
import { adminThemeOverrides, adminThemeTokens } from '../shared/theme/adminThemeOverrides'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect()

const collapsed = ref(false)
const activeKey = ref<AdminNavigationKey>('users')
const mobileMenuOpen = ref(false)
const isMobile = ref(false)
const isLoginRoute = computed(() => route.path === ADMIN_LOGIN_PATH)

const adminLayoutStyleVars = {
  '--admin-bg': adminThemeTokens.bg,
  '--admin-card': adminThemeTokens.cardTranslucent,
  '--admin-card-strong': adminThemeTokens.card,
  '--admin-border': adminThemeTokens.border,
  '--admin-border-soft': adminThemeTokens.borderSoft,
  '--admin-border-hover': adminThemeTokens.borderHover,
  '--admin-text-1': adminThemeTokens.textPrimary,
  '--admin-text-2': adminThemeTokens.textSecondary,
  '--admin-text-3': adminThemeTokens.textTertiary,
  '--admin-text-inverse': adminThemeTokens.textInverse,
  '--admin-title-color': adminThemeTokens.textPrimary,
  '--admin-subtitle-color': adminThemeTokens.textTertiary,
  '--admin-accent': adminThemeTokens.accent,
  '--admin-accent-soft': adminThemeTokens.accentSoft,
  '--admin-surface-muted': adminThemeTokens.surfaceMuted,
  '--admin-surface-hover': adminThemeTokens.surfaceHover,
  '--admin-surface-pressed': adminThemeTokens.surfacePressed,
  '--admin-control-bg': adminThemeTokens.controlBg,
  '--admin-control-bg-hover': adminThemeTokens.controlBgHover,
  '--admin-control-border': adminThemeTokens.controlBorder,
  '--admin-control-border-hover': adminThemeTokens.controlBorderHover,
  '--admin-nav-active-bg': adminThemeTokens.navActiveBg,
  '--admin-nav-active-bg-hover': adminThemeTokens.navActiveBgHover,
} as Record<string, string>

watch(
  () => route.path,
  (path) => {
    const item = findAdminNavigationItemByPath(path)
    if (item) {
      activeKey.value = item.key
    }
  },
  { immediate: true },
)

const renderMenuIcon = (iconComp: any) => () =>
  h(NIcon, { class: 'menu-icon', size: 22 }, { default: () => h(iconComp) })

const menuOptions = adminNavigationItems.map((item) => ({
  label: item.label,
  key: item.key,
  icon: renderMenuIcon(item.icon),
})) as any

const currentMeta = computed(() => {
  return adminNavigationByKey[activeKey.value] || { title: '管理后台' }
})

function handleMenuUpdate(key: string) {
  const item = adminNavigationByKey[key as AdminNavigationKey]
  if (item) {
    activeKey.value = item.key
    void router.push(item.path)
  }
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

async function handleUserAction(key: string) {
  if (key === 'logout') {
    await logoutAndRedirect('/admin/login')
  } else if (key === 'profile') {
    await router.push('/user/center')
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
  background: var(--admin-bg);
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
  width: 196px;
  height: 100%;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--admin-border);
  border-radius: 16px;
  background: var(--admin-card);
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
  justify-content: center;
  border-bottom: 1px solid var(--admin-border-soft);
}

.brand-copy {
  min-width: 0;
  width: 100%;
  display: grid;
  place-items: center;
  text-align: center;
}

.brand-title {
  margin: 0;
  font-size: 24px;
  font-weight: 800;
  line-height: 1;
  color: var(--admin-title-color);
  white-space: nowrap;
}

.admin-shell-main {
  min-width: 0;
  min-height: 0;
  border: 1px solid var(--admin-border);
  border-radius: 16px;
  background: var(--admin-card-strong);
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
  border-bottom: 1px solid var(--admin-border-soft);
}

.header-left,
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.shell-action-btn {
  --n-border: none;
  --n-border-hover: none;
  --n-border-pressed: none;
  --n-border-focus: none;
  --n-color: transparent;
  --n-color-hover: transparent;
  --n-color-pressed: transparent;
  --n-color-focus: transparent;
  font-size: 16px;
  color: var(--admin-text-2);
  border: 0;
  background: transparent;
  box-shadow: none;
  transition: color 0.18s ease;
}

.shell-action-btn:hover {
  color: var(--admin-text-1);
}

.title-wrap {
  min-width: 0;
}

.page-title {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  color: var(--admin-title-color);
}

.user-badge {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 4px 10px 4px 4px;
  border-radius: 999px;
  border: 1px solid var(--admin-border);
  background: rgba(255, 255, 255, 0.9);
  transition:
    border-color 0.18s ease,
    background 0.18s ease;
}

.user-badge:hover {
  background: var(--admin-surface-hover);
  border-color: var(--admin-border-hover);
}

.user-name {
  color: var(--admin-text-1);
  font-size: 13px;
  max-width: 120px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.user-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  overflow: hidden;
  border: 1px solid var(--admin-border-soft);
  background: var(--admin-surface-muted);
  color: var(--admin-text-2);
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  transition:
    border-color 0.18s ease,
    background 0.18s ease,
    color 0.18s ease;
}

.user-badge:hover .user-avatar {
  border-color: var(--admin-border-hover);
  background: var(--admin-surface-hover);
  color: var(--admin-text-1);
}

.user-avatar img,
.user-avatar-placeholder {
  width: 100%;
  height: 100%;
  border-radius: inherit;
}

.user-avatar img {
  display: block;
  object-fit: cover;
}

.user-avatar-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
}

.user-avatar-icon {
  width: 17px;
  height: 17px;
  stroke-width: 2;
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

:deep(.admin-nav-menu) {
  padding: 10px 8px;
  gap: 4px;
}

.admin-shell-sider :deep(.admin-nav-menu) {
  flex: 1;
  overflow-y: auto;
  overscroll-behavior: contain;
}

:deep(.admin-nav-menu .n-menu-item-group-title) {
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-size: 11px;
}

:deep(
  .admin-nav-menu
    :is(
      .n-menu-item-content,
      .n-menu-item-content-header,
      .n-menu-item-content-header a,
      .n-menu-item-group-title
    )
) {
  color: var(--admin-text-1);
  font-weight: 400;
}

.menu-icon {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  font-size: 22px;
  line-height: 1;
}

:deep(.admin-mobile-drawer .n-drawer-content) {
  --n-color: var(--admin-card-strong);
  --n-border-color: var(--admin-border);
}

:deep(.admin-mobile-drawer .n-drawer-header) {
  border-bottom: 1px solid var(--admin-border-soft);
}

:deep(.admin-mobile-drawer .n-drawer-header__main) {
  color: var(--admin-text-1);
  font-weight: 700;
  letter-spacing: 0.02em;
}

:deep(.admin-mobile-drawer .n-drawer-body-content-wrapper) {
  background: var(--admin-bg);
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
