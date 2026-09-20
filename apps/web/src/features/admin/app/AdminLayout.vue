<template>
  <router-view v-if="isLoginRoute" />
  <div v-else class="flex min-h-screen bg-ib-page text-ib-text">
    <button
      v-if="drawerOpen"
      type="button"
      aria-label="关闭导航"
      class="fixed inset-0 z-40 bg-black/30 backdrop-blur-xs lg:hidden"
      @click="drawerOpen = false"
    />

    <!-- Desktop spacer keeps content width stable; collapsed hover expands as overlay -->
    <div
      class="hidden shrink-0 transition-[width] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] lg:block"
      :class="isCollapsed ? 'w-14' : 'w-52'"
    />

    <aside
      ref="siderRef"
      aria-label="后台导航"
      class="fixed top-0 left-0 z-50 flex h-screen shrink-0 flex-col justify-between overflow-hidden border-r border-admin-border bg-admin-bg transition-[width,transform,box-shadow] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] lg:z-30"
      :class="[
        effectiveCollapsed ? 'w-52 lg:w-14' : 'w-52',
        isCollapsed && isHovered ? 'lg:z-40' : '',
        drawerOpen ? 'translate-x-0 shadow-xl' : '-translate-x-full lg:translate-x-0',
      ]"
      @mouseleave="handleNavMouseLeave"
      @focusout="handleSiderFocusOut"
    >
      <div
        class="flex min-h-0 flex-1 flex-col"
        @mouseenter="handleNavMouseEnter"
        @mouseleave="handleNavMouseLeave"
        @focusin="handleNavMouseEnter"
        @focusout="handleSiderFocusOut"
      >
        <div class="flex h-14 shrink-0 items-center border-b border-admin-border px-2">
          <router-link
            :to="ADMIN_DEFAULT_PATH"
            :aria-label="brandAriaLabel"
            :title="effectiveCollapsed ? projectName || 'IdeaBoard' : undefined"
            class="group flex h-10 w-full min-w-0 items-center overflow-hidden rounded-xl whitespace-nowrap transition-colors hover:bg-admin-surface-muted active:scale-[0.98]"
          >
            <span
              class="flex size-10 shrink-0 items-center justify-center transition-transform duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] group-hover:scale-105"
            >
              <SiteBrandLogo
                :src="logoURL"
                :fallback-text="fallbackText"
                :class="brandMarkClasses.logo"
              />
            </span>
            <span
              class="shrink-0 truncate pl-1.5 text-lg font-bold tracking-tight text-admin-title-color transition-[opacity,transform]"
              :class="[
                effectiveCollapsed
                  ? 'pointer-events-none opacity-0 -translate-x-3 duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]'
                  : 'opacity-100 translate-x-0 delay-75 duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]',
              ]"
            >
              {{ projectName || 'IdeaBoard' }}
            </span>
          </router-link>
        </div>

        <nav
          class="flex min-h-0 flex-1 flex-col gap-1.5 overflow-x-hidden overflow-y-auto p-2"
          aria-label="后台功能"
        >
          <div
            v-for="(group, groupIndex) in adminNavigationGroups"
            :key="group.title"
            class="flex flex-col gap-1"
          >
            <div class="flex flex-col">
              <Transition
                enter-active-class="transition-[grid-template-rows,opacity] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
                enter-from-class="grid-rows-[0fr] opacity-0"
                enter-to-class="grid-rows-[1fr] opacity-100"
                leave-active-class="transition-[grid-template-rows,opacity] duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
                leave-from-class="grid-rows-[1fr] opacity-100"
                leave-to-class="grid-rows-[0fr] opacity-0"
              >
                <div v-if="groupIndex > 0 && effectiveCollapsed" class="grid">
                  <div class="min-h-0 overflow-hidden">
                    <div class="mx-2 my-1 h-px bg-admin-border-soft" aria-hidden="true" />
                  </div>
                </div>
              </Transition>

              <Transition
                enter-active-class="transition-[grid-template-rows,opacity,transform] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
                enter-from-class="grid-rows-[0fr] opacity-0 -translate-x-3 -translate-y-2 scale-95"
                enter-to-class="grid-rows-[1fr] opacity-100 translate-x-0 translate-y-0 scale-100"
                leave-active-class="transition-[grid-template-rows,opacity,transform] duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
                leave-from-class="grid-rows-[1fr] opacity-100 translate-x-0 translate-y-0 scale-100"
                leave-to-class="grid-rows-[0fr] opacity-0 -translate-x-3 -translate-y-2 scale-95"
              >
                <div v-if="!effectiveCollapsed" class="grid origin-top-left">
                  <div class="min-h-0 overflow-hidden">
                    <div
                      class="px-3 pt-2 pb-1 text-xs font-semibold tracking-wider text-admin-text-3 select-none whitespace-nowrap"
                    >
                      {{ group.title }}
                    </div>
                  </div>
                </div>
              </Transition>
            </div>

            <router-link
              v-for="item in itemsOfGroup(group)"
              :key="item.key"
              :to="item.path"
              :title="effectiveCollapsed ? `${group.title} · ${item.label}` : item.label"
              :aria-label="item.label"
              :aria-current="activeKey === item.key ? 'page' : undefined"
              class="group flex h-10 w-full items-center overflow-hidden rounded-xl text-sm font-medium whitespace-nowrap text-admin-title-color transition-colors hover:bg-admin-surface-muted active:scale-[0.98]"
              @click="drawerOpen = false"
            >
              <span class="flex size-10 shrink-0 items-center justify-center">
                <component
                  :is="item.icon"
                  :class="[
                    navIconSizeClass,
                    'text-admin-text-2 transition-transform group-hover:scale-110',
                    activeKey === item.key && 'text-admin-accent scale-105 font-bold',
                  ]"
                />
              </span>
              <span
                class="shrink-0 pl-1 origin-top-left transition-[opacity,transform]"
                :class="[
                  effectiveCollapsed
                    ? 'pointer-events-none opacity-0 -translate-x-3 -translate-y-1.5 scale-95 duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]'
                    : 'opacity-100 translate-x-0 translate-y-0 scale-100 delay-75 duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]',
                ]"
              >
                {{ item.label }}
              </span>
            </router-link>
          </div>
        </nav>
      </div>

      <div
        class="hidden h-14 shrink-0 items-center border-t border-admin-border px-2 lg:flex"
        @mouseenter="handleBottomMouseEnter"
        @focusin="handleBottomMouseEnter"
      >
        <button
          type="button"
          :aria-label="isCollapsed ? '展开并固定侧边栏' : '折叠侧边栏'"
          :title="isCollapsed ? '展开并固定侧边栏' : '折叠侧边栏'"
          class="group flex size-10 items-center justify-center rounded-xl text-admin-text-2 transition hover:bg-admin-surface-muted hover:text-admin-title-color active:scale-95 cursor-pointer"
          @click="toggleCollapsed"
        >
          <SidebarCollapseIcon
            class="size-[18px] transition-transform"
            :class="isCollapsed ? 'rotate-180' : 'rotate-0'"
          />
        </button>
      </div>
    </aside>

    <div class="flex min-h-screen min-w-0 flex-1 flex-col">
      <!-- 顶栏组：一级主顶栏 + 二级导航栏统一常驻吸顶 -->
      <div class="sticky top-0 z-30 flex shrink-0 flex-col bg-admin-bg">
        <header
          class="flex h-14 shrink-0 items-center justify-between border-b border-admin-border px-4 sm:px-6 lg:px-8"
        >
          <div class="flex min-w-0 items-center gap-3">
            <button
              type="button"
              aria-label="切换导航菜单"
              class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-admin-border text-admin-text-1 transition-colors hover:bg-admin-surface-muted lg:hidden"
              @click="drawerOpen = !drawerOpen"
            >
              <MenuIcon v-if="!drawerOpen" class="size-5" />
              <Xmark v-else class="size-5" />
            </button>

            <button
              type="button"
              title="回到顶部"
              class="hidden min-w-0 items-center gap-2 text-left transition lg:flex"
              :class="
                stickyTitleVisible
                  ? 'translate-y-0 opacity-100'
                  : 'pointer-events-none -translate-y-1 opacity-0'
              "
              @click="scrollToTop"
            >
              <component
                :is="activeItem.icon"
                :class="[navIconSizeClass, 'shrink-0 text-admin-text-2']"
                aria-hidden="true"
              />
              <span class="truncate text-base font-semibold text-admin-title-color">
                {{ dynamicTitle || currentMeta.title }}
              </span>
            </button>
          </div>

          <div class="flex shrink-0 items-center gap-2">
            <PopoverRoot>
              <PopoverTrigger as-child>
                <button
                  title="异常雷达"
                  type="button"
                  :class="[notificationButtonClass, 'cursor-pointer']"
                  :aria-label="hasAnomalies ? `异常雷达，${anomalyCount} 项活跃异常` : '异常雷达'"
                  aria-haspopup="dialog"
                >
                  <BellIcon :class="shellIconClass" />
                  <span
                    v-if="hasAnomalies"
                    class="absolute top-[7px] right-[7px] size-2.5 rounded-full bg-admin-danger ring-2 ring-white"
                    aria-hidden="true"
                  ></span>
                </button>
              </PopoverTrigger>
              <PopoverPortal>
                <PopoverContent
                  class="z-50 w-[340px] max-w-[calc(100vw_-_28px)] rounded-xl border border-admin-border bg-admin-card-strong p-0 text-admin-text-1 shadow-lg outline-none overflow-hidden data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2"
                  :side-offset="8"
                  align="end"
                >
                  <div :class="adminAnomalyPopoverClass">
                    <div
                      class="flex items-start justify-between gap-4 border-b border-admin-border px-4 py-3.5"
                    >
                      <div class="min-w-0">
                        <div class="text-admin-content font-bold text-admin-title-color">
                          异常雷达
                        </div>
                      </div>
                      <AdminBadge
                        :value="hasAnomalies ? `${anomalyCount} 项` : '正常'"
                        :severity="hasAnomalies ? 'danger' : 'success'"
                        rounded
                      />
                    </div>

                    <div
                      v-if="hasAnomalies"
                      class="app-scrollbar min-h-0 flex-1 overflow-y-auto p-2"
                    >
                      <button
                        v-for="item in anomalies"
                        :key="item.id"
                        type="button"
                        class="flex w-full min-w-0 cursor-pointer gap-3 rounded-xl px-3 py-3 text-left transition-colors hover:bg-admin-surface-muted"
                        @click="handleAnomalySelect(item)"
                      >
                        <span
                          class="mt-[7px] size-2 shrink-0 rounded-full bg-admin-danger"
                          aria-hidden="true"
                        ></span>
                        <span class="min-w-0 flex-1">
                          <span class="flex min-w-0 items-center justify-between gap-3">
                            <span class="truncate text-admin-body font-bold text-admin-text-1">
                              {{ item.title }}
                            </span>
                            <span class="shrink-0 text-admin-caption font-bold text-admin-text-3">
                              {{ item.metric || item.actionLabel }}
                            </span>
                          </span>
                          <span class="mt-1 block text-admin-meta leading-5 text-admin-text-2">
                            {{ item.summary }}
                          </span>
                          <span class="mt-1.5 block text-admin-caption font-bold text-admin-text-3">
                            {{ item.actionLabel }}
                          </span>
                        </span>
                      </button>
                    </div>

                    <div v-else class="px-5 py-8 text-center">
                      <div class="text-admin-content font-bold text-admin-title-color">
                        运行正常
                      </div>
                      <div class="mt-1.5 text-admin-meta text-admin-subtitle-color">
                        当前没有活跃异常
                      </div>
                    </div>
                  </div>
                </PopoverContent>
              </PopoverPortal>
            </PopoverRoot>

            <PopoverRoot v-model:open="userMenuOpen">
              <PopoverTrigger as-child>
                <button
                  title="账户菜单"
                  type="button"
                  :class="userAvatarButtonClass"
                  aria-label="账户菜单"
                  aria-haspopup="menu"
                  :aria-expanded="userMenuOpen"
                >
                  <img
                    v-if="userStore.user?.avatar"
                    :src="userStore.user.avatar"
                    :class="userAvatarImageClass"
                    alt="用户头像"
                  />
                  <span v-else :class="userAvatarPlaceholderClass" aria-hidden="true">
                    <UserIcon :class="userAvatarIconClass" />
                  </span>
                </button>
              </PopoverTrigger>
              <PopoverPortal>
                <PopoverContent
                  class="z-50 min-w-[200px] rounded-xl border border-admin-border bg-admin-card-strong p-1.5 text-admin-text-1 shadow-lg outline-none select-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=right]:slide-in-from-left-2"
                  :side-offset="8"
                  side="bottom"
                  align="end"
                >
                  <div class="flex flex-col" role="menu">
                    <div
                      class="flex items-center gap-2.5 px-3 py-2 border-b border-admin-border-soft mb-1"
                    >
                      <div
                        class="flex size-7.5 shrink-0 items-center justify-center rounded-lg bg-admin-surface-muted text-admin-caption font-black text-admin-accent border border-admin-border-soft"
                      >
                        {{ (userStore.user?.display_name || 'A').substring(0, 1).toUpperCase() }}
                      </div>
                      <div class="flex min-w-0 flex-col">
                        <span class="truncate text-xs font-bold text-admin-title-color">
                          {{ userStore.user?.display_name || 'Administrator' }}
                        </span>
                        <span class="truncate text-[11px] font-medium text-admin-text-3 font-mono">
                          {{ userStore.user?.role === 'admin' ? '系统管理员' : 'Administrator' }}
                        </span>
                      </div>
                    </div>

                    <button
                      type="button"
                      role="menuitem"
                      class="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-semibold text-admin-text-2 hover:bg-admin-surface-hover hover:text-admin-title-color transition-colors cursor-pointer"
                      @click="handleUserAction('boards')"
                    >
                      <ViewGrid class="size-4 text-admin-text-3" />
                      <span>返回画布</span>
                    </button>

                    <button
                      type="button"
                      role="menuitem"
                      class="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-semibold text-admin-text-2 hover:bg-admin-danger-soft hover:text-admin-danger transition-colors cursor-pointer"
                      @click="handleUserAction('logout')"
                    >
                      <LogOut class="size-4" />
                      <span>退出登录</span>
                    </button>
                  </div>
                </PopoverContent>
              </PopoverPortal>
            </PopoverRoot>
          </div>
        </header>
        <!-- 二级子导航栏（插槽/Teleport 挂载区：有子页面时挂载，无子页面时 empty:hidden 高度折叠） -->
        <div id="admin-sub-header" class="border-b border-admin-border empty:hidden"></div>
      </div>

      <!-- 纯净的主内容容器：统一 pt-6 pb-8 标准留白 -->
      <main
        class="mx-auto flex w-full min-w-0 max-w-[1600px] flex-1 flex-col px-4 pt-6 pb-8 sm:px-6 lg:px-8"
      >
        <!-- hidePageHeader 路由（如积分运营自带吸顶标题块）：v-show 保留 DOM，
             顶栏滚动标题照常从 h1 取词 -->
        <div
          v-show="!hidePageHeader"
          class="mb-6 flex shrink-0 flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"
        >
          <div class="min-w-0">
            <h1 class="m-0 text-2xl font-bold tracking-wide text-admin-title-color">
              {{ currentMeta.title }}
            </h1>
            <p class="mt-1 text-sm text-admin-subtitle-color">
              {{ currentMeta.subtitle }}
            </p>
          </div>
          <div
            id="admin-header-actions"
            class="flex shrink-0 items-center gap-2 empty:hidden"
          ></div>
        </div>
        <router-view />
      </main>
    </div>

    <AdminConfirmDialog />
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '../../auth/session/userSession'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import AdminConfirmDialog from '../shared/components/AdminConfirmDialog.vue'
import AdminBadge from '../shared/components/AdminBadge.vue'
import {
  BellNotificationSolid as BellIcon,
  LogOut,
  Menu as MenuIcon,
  SidebarCollapse as SidebarCollapseIcon,
  User as UserIcon,
  ViewGrid,
  Xmark,
} from '@iconoir/vue'
import {
  adminNavigationByKey,
  adminNavigationGroups,
  ADMIN_DEFAULT_PATH,
  findAdminNavigationItemByPath,
  resolveAdminPageTitle,
  type AdminNavigationGroup,
  type AdminNavigationItem,
} from './navigation'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import {
  ADMIN_CREDITS_PATH,
  ADMIN_LOGIN_PATH,
  ADMIN_SETTINGS_PATH,
  USER_BOARDS_PATH,
  isPathUnderRoute,
} from '../../../shared/constants/routePaths'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { brandMarkClasses } from '../../../shared/lib/brandMark'
import { useAnomalyRadar } from '../shared/composables/useAnomalyRadar'
import { useAdminToast } from '../shared/lib/toast'
import { useBrandLogoOverrides } from '../shared/logos/useBrandLogoOverrides'
import type { AdminAnomalyItem } from '../shared/lib/anomalyRules'
import { adminAnomalyPopoverClass } from '../shared/lib/uiClasses'

const toast = useAdminToast()

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })

const drawerOpen = ref(false)
const userMenuOpen = ref(false)
const isCollapsed = ref(false)
const isHovered = ref(false)
let hoverLeaveTimer: ReturnType<typeof setTimeout> | null = null
let hoverEnterTimer: ReturnType<typeof setTimeout> | null = null

function clearHoverTimers() {
  if (hoverLeaveTimer) {
    clearTimeout(hoverLeaveTimer)
    hoverLeaveTimer = null
  }
  if (hoverEnterTimer) {
    clearTimeout(hoverEnterTimer)
    hoverEnterTimer = null
  }
}

function handleNavMouseEnter() {
  clearHoverTimers()
  if (!isCollapsed.value) return
  hoverEnterTimer = setTimeout(() => {
    isHovered.value = true
    hoverEnterTimer = null
  }, 150)
}

function handleNavMouseLeave() {
  clearHoverTimers()
  if (!isHovered.value) return
  hoverLeaveTimer = setTimeout(() => {
    isHovered.value = false
    hoverLeaveTimer = null
  }, 120)
}

function handleBottomMouseEnter() {
  clearHoverTimers()
  isHovered.value = false
}

const effectiveCollapsed = computed(() => isCollapsed.value && !isHovered.value)
const showStickyTitle = ref(false)
const dynamicTitle = ref('')
// 积分运营与系统设置等二级导航聚合页常驻顶栏标题（不随滚动显隐，其它页保持滚动行为）。
const pinStickyTitle = computed(
  () =>
    isPathUnderRoute(route.path, ADMIN_CREDITS_PATH) ||
    isPathUnderRoute(route.path, ADMIN_SETTINGS_PATH),
)
const stickyTitleVisible = computed(() => pinStickyTitle.value || showStickyTitle.value)
const isLoginRoute = computed(() => route.path === ADMIN_LOGIN_PATH)
const isAdminShellRoute = computed(() => !isLoginRoute.value)
const isAnomalyRadarEnabled = computed(() => isAdminShellRoute.value)
const { anomalies, anomalyCount, hasAnomalies } = useAnomalyRadar({
  enabled: isAnomalyRadarEnabled,
})
const { fallbackText, logoURL, projectName } = useSiteBranding({
  enabled: isAdminShellRoute,
})
useBrandLogoOverrides(isAdminShellRoute)
const brandAriaLabel = computed(() => `${projectName.value} Admin`)

const userAvatarButtonClass =
  'relative flex size-9 items-center justify-center rounded-lg text-admin-text-3 transition-colors hover:bg-admin-surface-muted cursor-pointer'
const userAvatarImageClass = 'size-6 rounded-full object-cover'
const userAvatarPlaceholderClass = 'flex size-full items-center justify-center text-admin-text-3'
const userAvatarIconClass = 'size-4'

// Nav, topbar and avatar glyphs share one size token.
const navIconSizeClass = 'size-4'
const shellIconClass = navIconSizeClass

const notificationButtonClass =
  'relative flex size-9 items-center justify-center rounded-lg text-admin-text-3 transition-colors hover:bg-admin-surface-muted'

const activeItem = computed(() => {
  return findAdminNavigationItemByPath(route.path) || adminNavigationByKey.statistics
})

const activeKey = computed(() => activeItem.value.key)

const currentMeta = computed(() => {
  return {
    ...activeItem.value,
    title: resolveAdminPageTitle(route.path),
  }
})

// 子路由可经 meta.hidePageHeader 藏起本标题块（如自带吸顶标题的聚合页）。
const hidePageHeader = computed(() => route.matched.some((record) => record.meta.hidePageHeader))

function itemsOfGroup(group: AdminNavigationGroup): AdminNavigationItem[] {
  return group.keys
    .map((key) => adminNavigationByKey[key])
    .filter((item): item is AdminNavigationItem => Boolean(item))
}

function toggleCollapsed() {
  clearHoverTimers()
  isHovered.value = false
  isCollapsed.value = !isCollapsed.value
  try {
    localStorage.setItem('admin_sidebar_collapsed', String(isCollapsed.value))
  } catch {
    // ignore private-mode storage failures
  }
}

function scrollToTop() {
  if (typeof window !== 'undefined') {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
}

function updateStickyTitle() {
  if (typeof window === 'undefined') return
  if (window.scrollY <= 10) {
    showStickyTitle.value = false
    return
  }
  showStickyTitle.value = true
  if (typeof document === 'undefined') return
  const heading = document.querySelector('main h1')
  const text = heading?.textContent?.replace(/\s+/g, ' ').trim()
  dynamicTitle.value = text || ''
}

function handleUserAction(action: 'boards' | 'logout') {
  userMenuOpen.value = false
  if (action === 'boards') {
    void router.push(USER_BOARDS_PATH)
    return
  }
  void logoutAndRedirect(ADMIN_LOGIN_PATH, { replace: true })
}

function handleAnomalySelect(item: AdminAnomalyItem) {
  if (route.path === item.route) {
    toast.info(`已在当前页面：${item.title}`)
    return
  }
  void router.push(item.route)
}

function handleKeyDown(event: KeyboardEvent) {
  if (event.key === 'Escape' && drawerOpen.value) {
    drawerOpen.value = false
  }
}

const siderRef = ref<HTMLElement | null>(null)

function handleSiderFocusOut(event: FocusEvent) {
  const next = event.relatedTarget
  if (next instanceof HTMLElement && siderRef.value?.contains(next)) return
  clearHoverTimers()
  isHovered.value = false
}

let scrollTicking = false
function handleScroll() {
  if (scrollTicking) return
  scrollTicking = true
  requestAnimationFrame(() => {
    updateStickyTitle()
    scrollTicking = false
  })
}

onMounted(() => {
  try {
    const saved = localStorage.getItem('admin_sidebar_collapsed')
    if (saved !== null) isCollapsed.value = saved === 'true'
  } catch {
    // ignore private-mode storage failures
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('scroll', handleScroll, { passive: true })
    window.addEventListener('resize', handleScroll, { passive: true })
    window.addEventListener('keydown', handleKeyDown)
  }
  updateStickyTitle()
})

onUnmounted(() => {
  clearHoverTimers()
  if (typeof window !== 'undefined') {
    window.removeEventListener('scroll', handleScroll)
    window.removeEventListener('resize', handleScroll)
    window.removeEventListener('keydown', handleKeyDown)
  }
})

watch(
  () => route.path,
  () => {
    clearHoverTimers()
    isHovered.value = false
    drawerOpen.value = false
    userMenuOpen.value = false
    showStickyTitle.value = false
    dynamicTitle.value = ''
  },
)
</script>
