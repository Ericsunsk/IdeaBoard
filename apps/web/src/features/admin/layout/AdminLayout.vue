<template>
  <n-config-provider :theme-overrides="adminThemeOverrides">
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
                <n-button quaternary circle class="shell-action-btn" @click="handleToggleMenu">
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
  type GlobalThemeOverrides,
} from 'naive-ui'
import {
  PeopleOutline,
  LinkOutline,
  ChatbubbleEllipsesOutline,
  CloudOutline,
  SpeedometerOutline,
} from '@vicons/ionicons5'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const collapsed = ref(false)
const activeKey = ref('users')
const mobileMenuOpen = ref(false)
const isMobile = ref(false)
const isLoginRoute = computed(() => route.path === '/admin/login')

const adminThemeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#f4f4f5',
    primaryColorHover: '#ffffff',
    primaryColorPressed: '#d4d4d8',
    primaryColorSuppl: '#f4f4f5',
  },
  Button: {
    colorPrimary: '#f4f4f5',
    colorHoverPrimary: '#ffffff',
    colorPressedPrimary: '#d4d4d8',
    colorFocusPrimary: '#ffffff',
    textColorPrimary: '#111111',
    textColorHoverPrimary: '#111111',
    textColorPressedPrimary: '#111111',
    textColorFocusPrimary: '#111111',
    borderPrimary: '1px solid #f4f4f5',
    borderHoverPrimary: '1px solid #ffffff',
    borderPressedPrimary: '1px solid #d4d4d8',
    borderFocusPrimary: '1px solid #ffffff',
    textColorTextPrimary: '#f4f4f5',
    textColorTextHoverPrimary: '#ffffff',
    textColorTextPressedPrimary: '#d4d4d8',
    textColorTextFocusPrimary: '#ffffff',
    textColorGhostPrimary: '#f4f4f5',
    textColorGhostHoverPrimary: '#ffffff',
    textColorGhostPressedPrimary: '#d4d4d8',
    textColorGhostFocusPrimary: '#ffffff',
  },
  Card: {
    colorModal: '#101113',
    borderColor: '#232326',
    titleTextColor: '#f4f4f5',
    textColor: '#a1a1aa',
    actionColor: '#151618',
    borderRadius: '18px',
    closeIconColor: '#d4d4d8',
    closeIconColorHover: '#ffffff',
    closeIconColorPressed: '#f4f4f5',
    closeColorHover: 'rgba(255, 255, 255, 0.08)',
    closeColorPressed: 'rgba(255, 255, 255, 0.12)',
    boxShadow: '0 28px 80px rgba(0, 0, 0, 0.42)',
  },
  Dialog: {
    color: '#101113',
    titleTextColor: '#f4f4f5',
    textColor: '#a1a1aa',
    border: '1px solid #232326',
    borderRadius: '18px',
    closeIconColor: '#d4d4d8',
    closeIconColorHover: '#ffffff',
    closeIconColorPressed: '#f4f4f5',
    closeColorHover: 'rgba(255, 255, 255, 0.08)',
    closeColorPressed: 'rgba(255, 255, 255, 0.12)',
    iconColor: '#f4f4f5',
    iconColorInfo: '#f4f4f5',
    iconColorSuccess: '#4ade80',
    iconColorWarning: '#fbbf24',
    iconColorError: '#fb7185',
  },
  Input: {
    boxShadowFocus: 'none',
    borderFocus: '1px solid #3f3f46',
    borderHover: '1px solid #27272a',
  },
  Select: {
    boxShadowActive: 'none',
    boxShadowFocus: 'none',
    borderActive: '1px solid #3f3f46',
    borderFocus: '1px solid #3f3f46',
    borderHover: '1px solid #27272a',
  },
  InputNumber: {
    boxShadowFocus: 'none',
    borderFocus: '1px solid #3f3f46',
    borderHover: '1px solid #27272a',
  },
  InternalSelectMenu: {
    color: '#101113',
    optionTextColor: '#a1a1aa',
    optionTextColorActive: '#f4f4f5',
    optionColorActive: 'rgba(255, 255, 255, 0.04)',
    optionColorPending: 'rgba(255, 255, 255, 0.04)',
  },
}

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
    icon: PeopleOutline,
  },
  {
    key: 'channels',
    label: '渠道商管理',
    title: '渠道商管理',
    icon: LinkOutline,
  },
  {
    key: 'statistics',
    label: '数据仪表盘',
    title: '数据仪表盘',
    icon: SpeedometerOutline,
  },
  {
    key: 'feature-prompts',
    label: '系统提示词',
    title: '系统提示词',
    icon: ChatbubbleEllipsesOutline,
  },
  {
    key: 'oss-config',
    label: '系统存储',
    title: '系统存储',
    icon: CloudOutline,
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
  background: #0a0a0b;
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
  border: 1px solid #232326;
  border-radius: 16px;
  background: #111214;
  transition: width 0.24s ease;
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
  border-bottom: 1px solid #232326;
}

.brand-area:hover {
  background: #141517;
}

.brand-mark {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  font-weight: 700;
  color: #f4f4f5;
  background: #18191b;
  border: 1px solid #2b2c2f;
}

.brand-title {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: #f4f4f5;
  letter-spacing: 0.02em;
}

.admin-shell-main {
  min-width: 0;
  min-height: 0;
  border: 1px solid #232326;
  border-radius: 16px;
  background: #101113;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.admin-shell-header {
  height: 68px;
  padding: 0 22px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #232326;
}

.header-left,
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.shell-action-btn {
  font-size: 16px;
  color: #d4d4d8;
  border: 1px solid #2b2c2f;
  background: #17181a;
  transition:
    border-color 0.18s ease,
    background 0.18s ease;
}

.shell-action-btn:hover {
  border-color: #3a3b40;
  background: #1c1d20;
}

.title-wrap {
  min-width: 0;
}

.page-title {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  color: #f4f4f5;
}

.user-badge {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 8px 10px;
  border-radius: 999px;
  border: 1px solid #2b2c2f;
  background: #151618;
  transition:
    border-color 0.18s ease,
    background 0.18s ease;
}

.user-badge:hover {
  background: #1a1b1e;
  border-color: #3a3b40;
}

.user-badge span {
  color: #f4f4f5;
  font-size: 13px;
  max-width: 120px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.user-avatar {
  background: #f4f4f5;
  color: #111111;
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
  scrollbar-color: rgba(255, 255, 255, 0.2) transparent;
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
  background: rgba(255, 255, 255, 0.22);
  background-clip: padding-box;
}

.admin-shell-content::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.32);
  background-clip: padding-box;
}

.admin-shell-content::-webkit-scrollbar-corner {
  background: transparent;
}

:deep(.n-menu) {
  --n-item-text-color: #8d8d93;
  --n-item-text-color-active: #f4f4f5;
  --n-item-text-color-hover: #f4f4f5;
  --n-item-color-active: #18191b;
  --n-item-color-hover: #151618;
  --n-item-icon-color: #67676d;
  --n-item-icon-color-active: #f4f4f5;
  --n-item-icon-color-hover: #d4d4d8;
  --n-group-header-text-color: #5a5a60;
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
  --n-color: #101113;
  --n-border-color: #232326;
}

:deep(.admin-mobile-drawer .n-drawer-header) {
  border-bottom: 1px solid #232326;
}

:deep(.admin-mobile-drawer .n-drawer-header__main) {
  color: #f4f4f5;
  font-weight: 700;
  letter-spacing: 0.02em;
}

:deep(.admin-mobile-drawer .n-drawer-body-content-wrapper) {
  background: #111214;
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
