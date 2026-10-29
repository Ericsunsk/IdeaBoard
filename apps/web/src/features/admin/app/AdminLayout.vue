<template>
  <div class="flex min-h-screen bg-background text-foreground">
    <button
      v-if="drawerOpen"
      type="button"
      aria-label="关闭导航"
      class="fixed inset-0 z-admin-overlay bg-scrim-soft backdrop-blur-xs lg:hidden"
      @click="drawerOpen = false"
    />

    <AdminSidebar ref="sidebarRef" v-model:drawer-open="drawerOpen" :active-key="activeKey" />

    <div class="flex min-h-screen min-w-0 flex-1 flex-col">
      <!-- 顶栏组：一级主顶栏 + 二级导航栏统一常驻吸顶 -->
      <AdminHeaderBar
        ref="headerBarRef"
        v-model:drawer-open="drawerOpen"
        :active-item="activeItem"
        :current-meta="currentMeta"
        :sticky-title-visible="stickyTitleVisible"
        :dynamic-title="dynamicTitle"
        @scroll-to-top="scrollToTop"
      />

      <!-- 纯净的主内容容器：统一 pt-6 pb-8 标准留白；fullWidth 页面解除 1600px 限制并按要求缩减右边距 1/2 -->
      <main
        class="mx-auto flex w-full min-w-0 flex-1 flex-col pt-6 pb-8"
        :class="[
          route.meta.fullWidth
            ? 'max-w-none px-4 sm:px-6 lg:pl-8 lg:pr-4'
            : 'max-w-admin-shell px-4 sm:px-6 lg:px-8',
        ]"
      >
        <!-- hidePageHeader 路由（如积分运营自带吸顶标题块）：v-show 保留 DOM，
             顶栏滚动标题照常从 h1 取词 -->
        <div
          v-show="!hidePageHeader"
          class="mb-6 flex shrink-0 flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"
        >
          <div class="min-w-0">
            <h1 class="m-0 text-2xl font-bold tracking-wide text-foreground">
              {{ currentMeta.title }}
            </h1>
            <p class="mt-1 text-sm text-muted-foreground">
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

    <ConfirmDialog surface="admin" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { authTransport, useUserStore } from '../../auth'
import ConfirmDialog from '@/core/components/ConfirmDialog.vue'
import AdminSidebar from './AdminSidebar.vue'
import AdminHeaderBar from './AdminHeaderBar.vue'
import {
  adminNavigationByKey,
  findAdminNavigationItemByPath,
  resolveAdminPageTitle,
} from './navigation'
import { ADMIN_CREDITS_PATH, isPathUnderRoute } from '../routePaths'
import { useBrandLogoOverrides } from '../shared'
import { useAdminStickyTitle } from './useAdminStickyTitle'

const route = useRoute()
const userStore = useUserStore()

const drawerOpen = ref(false)
const sidebarRef = ref<InstanceType<typeof AdminSidebar> | null>(null)
const headerBarRef = ref<InstanceType<typeof AdminHeaderBar> | null>(null)

const { showStickyTitle, dynamicTitle, reset: resetStickyTitle } = useAdminStickyTitle()

// 仅自带吸顶标题块的聚合页（hidePageHeader）常驻顶栏标题，其它单页遵循随滚动显隐的全局标准。
const pinStickyTitle = computed(
  () => Boolean(route.meta.hidePageHeader) || isPathUnderRoute(route.path, ADMIN_CREDITS_PATH),
)
const stickyTitleVisible = computed(() => pinStickyTitle.value || showStickyTitle.value)
useBrandLogoOverrides(true)

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

function scrollToTop() {
  if (typeof window !== 'undefined') {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
}

function handleKeyDown(event: KeyboardEvent) {
  if (event.key === 'Escape' && drawerOpen.value) {
    drawerOpen.value = false
  }
}

onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', handleKeyDown)
  }
  if (!userStore.email && import.meta.env.MODE !== 'test') {
    void authTransport
      .getSessionUser()
      .then((sessionUser) => {
        if (sessionUser?.email) userStore.setEmail(sessionUser.email)
      })
      .catch(() => {
        // Background session hydration must not block the admin shell.
      })
  }
})

onUnmounted(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', handleKeyDown)
  }
})

watch(
  () => route.path,
  () => {
    sidebarRef.value?.resetHoverState?.()
    headerBarRef.value?.closeUserMenu?.()
    drawerOpen.value = false
    resetStickyTitle()
  },
)
</script>
