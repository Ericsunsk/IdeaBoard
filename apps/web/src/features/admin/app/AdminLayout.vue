<template>
  <router-view v-if="isLoginRoute" />
  <div v-else :class="layoutShellClass">
    <aside :class="siderClass" aria-label="后台导航">
      <router-link :class="brandMarkClass" :to="ADMIN_DEFAULT_PATH" :aria-label="brandAriaLabel">
        <SiteBrandLogo
          :src="logoURL"
          :fallback-text="fallbackText"
          :class="brandMarkClasses.logo"
        />
      </router-link>

      <nav :class="navRailClass" aria-label="后台功能">
        <router-link
          v-for="item in adminNavigationItems"
          :key="item.key"
          v-tooltip.right="item.label"
          :to="item.path"
          :aria-label="item.label"
          :aria-current="activeKey === item.key ? 'page' : undefined"
          :class="[
            railButtonClass,
            activeKey === item.key ? railButtonActiveClass : railButtonIdleClass,
          ]"
        >
          <component :is="item.icon" :class="navIconClassFor(activeKey === item.key)" />
        </router-link>
      </nav>

      <div :class="siderBottomClass">
        <button
          v-tooltip.right="'账户菜单'"
          type="button"
          :class="userAvatarButtonClass"
          aria-label="账户菜单"
          aria-haspopup="menu"
          :aria-expanded="userMenuOpen"
          @click="toggleUserMenu"
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
      </div>
    </aside>

    <main :class="mainClass">
      <header :class="headerClass">
        <div class="flex min-w-0 items-start gap-3.5">
          <Button
            :class="mobileMenuButtonClass"
            type="button"
            text
            rounded
            aria-label="打开导航"
            @click="mobileMenuOpen = true"
          >
            <template #icon>
              <MenuIcon :class="shellIconClass" />
            </template>
          </Button>
          <div class="min-w-0 pt-0.5">
            <p
              class="m-0 mb-1 text-admin-body font-bold tracking-normal text-admin-text-2 max-[1080px]:hidden"
            >
              Overview
            </p>
            <h1
              class="m-0 text-admin-modal-title leading-[1.05] font-[780] tracking-normal text-admin-title-color max-[1080px]:text-2xl"
            >
              {{ currentMeta.title }}
            </h1>
          </div>
        </div>

        <div class="flex min-w-0 items-center gap-3.5">
          <button
            v-tooltip.bottom="'异常雷达'"
            :class="[notificationButtonClass, 'cursor-pointer']"
            type="button"
            :aria-label="hasAnomalies ? `异常雷达，${anomalyCount} 项活跃异常` : '异常雷达'"
            aria-haspopup="dialog"
            @click="toggleAnomalyRadar"
          >
            <BellIcon :class="shellIconClass" />
            <span
              v-if="hasAnomalies"
              class="absolute top-[7px] right-[7px] size-2.5 rounded-full bg-admin-danger ring-2 ring-white"
              aria-hidden="true"
            ></span>
          </button>
          <Popover ref="anomalyRadarPopoverRef" :pt="anomalyRadarPopoverPt">
            <div :class="adminAnomalyPopoverClass">
              <div
                class="flex items-start justify-between gap-4 border-b border-admin-border px-4 py-3.5"
              >
                <div class="min-w-0">
                  <div class="text-admin-content font-bold text-admin-title-color">异常雷达</div>
                </div>
                <Tag
                  :value="hasAnomalies ? `${anomalyCount} 项` : '正常'"
                  :severity="hasAnomalies ? 'danger' : 'success'"
                  rounded
                />
              </div>

              <div v-if="hasAnomalies" class="app-scrollbar min-h-0 flex-1 overflow-y-auto p-2">
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
                <div class="text-admin-content font-bold text-admin-title-color">运行正常</div>
                <div class="mt-1.5 text-admin-meta text-admin-subtitle-color">当前没有活跃异常</div>
              </div>
            </div>
          </Popover>
        </div>
      </header>

      <section :class="contentClass">
        <router-view />
      </section>
    </main>

    <Popover
      ref="userMenuRef"
      :pt="userMenuPt"
      @show="userMenuOpen = true"
      @hide="userMenuOpen = false"
    >
      <div class="flex min-w-[190px] flex-col p-1.5 select-none" role="menu">
        <div class="flex items-center gap-2.5 px-3 py-2">
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
          class="flex w-full items-center gap-2.5 rounded-xl px-3 py-2 text-xs font-semibold text-admin-text-2 hover:bg-admin-surface-hover hover:text-admin-title-color transition-colors cursor-pointer"
          @click="handleUserAction('boards')"
        >
          <ViewGrid class="size-4 text-admin-text-3" />
          <span>返回画布</span>
        </button>

        <button
          type="button"
          role="menuitem"
          class="flex w-full items-center gap-2.5 rounded-xl px-3 py-2 text-xs font-semibold text-admin-text-2 hover:bg-admin-danger-soft hover:text-admin-danger transition-colors cursor-pointer"
          @click="handleUserAction('logout')"
        >
          <LogOut class="size-4" />
          <span>退出登录</span>
        </button>
      </div>
    </Popover>
    <ConfirmDialog />

    <Drawer
      v-model:visible="mobileMenuOpen"
      position="left"
      :pt="mobileDrawerPt"
      :show-close-icon="false"
    >
      <template #container="{ closeCallback }">
        <div class="flex h-full flex-col gap-6 bg-admin-control-bg p-[22px]">
          <div class="flex items-center justify-between">
            <router-link
              :class="[brandMarkClass, '!ml-0']"
              :to="ADMIN_DEFAULT_PATH"
              :aria-label="brandAriaLabel"
              @click="closeCallback"
            >
              <SiteBrandLogo
                :src="logoURL"
                :fallback-text="fallbackText"
                :class="brandMarkClasses.logo"
              />
            </router-link>
            <Button
              :class="shellButtonClass"
              type="button"
              text
              rounded
              aria-label="关闭导航"
              @click="closeCallback"
            >
              <template #icon>
                <Xmark :class="shellIconClass" />
              </template>
            </Button>
          </div>

          <nav class="flex flex-col items-stretch gap-2" aria-label="后台功能">
            <router-link
              v-for="item in adminNavigationItems"
              :key="item.key"
              :to="item.path"
              :aria-current="activeKey === item.key ? 'page' : undefined"
              :class="[
                mobileNavButtonClass,
                activeKey === item.key ? mobileNavButtonActiveClass : mobileNavButtonIdleClass,
              ]"
              @click="closeCallback"
            >
              <component :is="item.icon" :class="navIconClassFor(activeKey === item.key)" />
              <span>{{ item.label }}</span>
            </router-link>
          </nav>
        </div>
      </template>
    </Drawer>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '../../auth/session/session'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import ConfirmDialog from 'primevue/confirmdialog'
import Drawer from 'primevue/drawer'
import Popover from 'primevue/popover'
import Tag from 'primevue/tag'
import Tooltip from 'primevue/tooltip'
import { usePrimeVue } from 'primevue/config'
import {
  Bell as BellIcon,
  LogOut,
  Menu as MenuIcon,
  User as UserIcon,
  ViewGrid,
  Xmark,
} from '@iconoir/vue'
import {
  adminNavigationByKey,
  adminNavigationItems,
  ADMIN_DEFAULT_PATH,
  ADMIN_LOGIN_PATH,
  findAdminNavigationItemByPath,
  resolveAdminPageTitle,
} from './navigation'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { USER_BOARDS_PATH } from '../../../shared/constants/routePaths'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { brandMarkClasses, createBrandMarkClass } from '../../../shared/lib/brandMark'
import { useAnomalyRadar } from '../shared/composables/useAnomalyRadar'
import { useAdminToast } from '../shared/lib/toast'
import { useBrandLogoOverrides } from '../shared/logos/useBrandLogoOverrides'
import type { AdminAnomalyItem } from '../shared/lib/anomalyRules'
import { createAdminPopoverPt } from '../shared/lib/overlayPt'
import { adminAnomalyPopoverClass } from '../shared/lib/uiClasses'
import { getActiveUserPrimeLocale } from '../../../shared/i18n'
import { ideaBoardAdminPrimeLocale } from './primeLocale'

const vTooltip = Tooltip
const toast = useAdminToast()

type PrimePopupMenu = {
  toggle: (event: Event) => void
}

type PrimePopupPanel = PrimePopupMenu & {
  hide?: () => void
}

const router = useRouter()
const route = useRoute()
const primeVue = usePrimeVue()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })

const mobileMenuOpen = ref(false)
const userMenuOpen = ref(false)
const userMenuRef = ref<PrimePopupPanel | null>(null)
const anomalyRadarPopoverRef = ref<PrimePopupPanel | null>(null)
const isLoginRoute = computed(() => route.path === ADMIN_LOGIN_PATH)
const isAdminShellRoute = computed(() => !isLoginRoute.value)
const isAnomalyRadarEnabled = computed(() => isAdminShellRoute.value)
const { anomalies, anomalyCount, hasAnomalies } = useAnomalyRadar({
  enabled: isAnomalyRadarEnabled,
})
const { fallbackText, logoURL, projectName } = useSiteBranding({
  enabled: isAdminShellRoute,
})
const adminBrandLogoOverridesQuery = useBrandLogoOverrides(isAdminShellRoute)
const brandAriaLabel = computed(() => `${projectName.value} Admin`)

// PrimeVue has one app-level locale. Admin deliberately retains its established Chinese date UI.
primeVue.config.locale = ideaBoardAdminPrimeLocale

const layoutShellClass =
  'grid min-h-screen grid-cols-[133px_minmax(0,1fr)] bg-ib-surface text-ib-text max-[1080px]:grid-cols-1'
const siderClass =
  "sticky top-0 flex h-screen self-start flex-col items-start bg-ib-surface py-[42px] max-[1080px]:hidden before:absolute before:top-[21px] before:bottom-[21px] before:left-[21px] before:box-border before:w-[84px] before:rounded-[var(--radius-ib-overlay)] before:bg-admin-control-bg before:content-['']"
const railButtonSizeClass = '!size-10 !min-w-[40px]'
const brandMarkClass = computed(() =>
  createBrandMarkClass(
    brandMarkClasses.sizes.compact,
    logoURL.value ? brandMarkClasses.states.image : brandMarkClasses.states.fallbackStrong,
    'relative z-[1] ml-[42px] focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-focus)]',
  ),
)
const navRailClass =
  'relative z-[1] ml-[21px] flex w-[84px] flex-1 flex-col items-center gap-3 pt-3.5'
const siderBottomClass = 'relative z-[1] ml-[21px] flex w-[84px] flex-col items-center gap-3'
const focusRingClass = 'focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]'
const railButtonClass = [
  '!ib-motion-control-colors !inline-flex !items-center !justify-center !rounded-[var(--radius-ib-control)] !border !border-transparent !text-ib-text-muted !no-underline !shadow-none',
  railButtonSizeClass,
  focusRingClass,
].join(' ')
const railButtonIdleClass = '!bg-transparent hover:!bg-ib-surface-hover hover:!text-ib-text'
const railButtonActiveClass =
  '!bg-ib-accent-pressed !text-ib-text-inverse !shadow-[0_8px_18px_rgba(0,0,0,0.15)]'
const userAvatarButtonClass = 'ib-round-control ib-user-avatar-button admin-user-avatar-button'
const userAvatarImageClass = 'ib-user-avatar-button__image'
const userAvatarPlaceholderClass = 'ib-user-avatar-button__placeholder'
const userAvatarIconClass = 'size-[17px]'
const shellButtonSkinClass = [
  '!ib-motion-control-colors !size-[42px] !items-center !justify-center !rounded-[9px] !text-ib-text-muted',
  'hover:!border-ib-border hover:!bg-ib-surface-hover hover:!text-ib-text',
  focusRingClass,
].join(' ')
const shellButtonClass = [
  '!inline-flex !bg-transparent !border !border-transparent !shadow-none',
  shellButtonSkinClass,
].join(' ')
const mobileMenuButtonClass = [
  '!hidden max-[1080px]:!inline-flex !bg-ib-surface !border !border-ib-border !shadow-none',
  shellButtonSkinClass,
].join(' ')
const notificationButtonClass = [
  '!inline-flex !bg-ib-surface !border !border-ib-border !shadow-[var(--shadow-ib-control)]',
  shellButtonSkinClass,
  'relative',
].join(' ')
const navIconIdleClass = 'size-[17px]'
const navIconActiveClass = 'size-[17px] !text-ib-text-inverse'
const shellIconClass = 'size-[19px]'
const mainClass = 'flex min-h-screen min-w-0 flex-col overflow-visible bg-ib-surface'
const headerClass =
  'flex min-h-[88px] items-start justify-between gap-6 bg-ib-surface py-[21px] pr-[42px] pl-0 max-[1080px]:min-h-[92px] max-[1080px]:items-center max-[1080px]:px-[22px] max-[1080px]:pt-[22px] max-[1080px]:pb-4 max-[720px]:gap-3 max-[720px]:px-[14px] max-[720px]:pt-4 max-[720px]:pb-3'
const contentClass =
  'min-h-0 flex-1 overflow-visible bg-ib-surface pr-[42px] pb-[38px] max-[1080px]:px-[22px] max-[1080px]:pb-7 max-[720px]:px-[14px] max-[720px]:pb-5'
const mobileNavButtonClass = [
  'ib-motion-control-colors flex min-h-12 w-full items-center justify-start gap-3 rounded-xl border border-transparent px-4 text-sm font-bold no-underline',
  focusRingClass,
].join(' ')
const mobileNavButtonIdleClass =
  'bg-transparent text-ib-text-muted hover:border-ib-border hover:bg-ib-surface hover:text-ib-text'
const mobileNavButtonActiveClass = 'bg-ib-accent-pressed text-ib-text-inverse'
const userMenuPt = createAdminPopoverPt({ contentClass: '!p-0' })
const anomalyRadarPopoverPt = createAdminPopoverPt({
  rootClass: '!max-h-[calc(100dvh-24px)] !overflow-hidden',
  contentClass: '!max-h-[calc(100dvh-24px)] !overflow-hidden !p-0',
})
const mobileDrawerPt = {
  root: {
    class: '!w-[min(320px,calc(100vw-32px))] !border-0 !bg-admin-control-bg',
  },
} as const

watch(
  () => route.path,
  () => {
    mobileMenuOpen.value = false
  },
)

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

function navIconClassFor(isActive: boolean) {
  return isActive ? navIconActiveClass : navIconIdleClass
}

function toggleUserMenu(event: Event) {
  userMenuOpen.value = !userMenuOpen.value
  userMenuRef.value?.toggle(event)
}

function toggleAnomalyRadar(event: Event) {
  anomalyRadarPopoverRef.value?.toggle(event)
}

function handleAnomalySelect(item: AdminAnomalyItem) {
  anomalyRadarPopoverRef.value?.hide?.()
  void router.push(item.route)
}

async function handleUserAction(key: 'boards' | 'logout') {
  userMenuRef.value?.hide?.()
  if (key === 'logout') {
    await logoutAndRedirect(ADMIN_LOGIN_PATH)
  } else {
    await router.push(USER_BOARDS_PATH)
  }
}

function setAdminRouteClass(enabled: boolean) {
  document.documentElement.classList.toggle('admin-route', enabled)
  document.body.classList.toggle('admin-route', enabled)
}

watch(
  () => adminBrandLogoOverridesQuery.error.value,
  (error) => {
    if (error) toast.warning('读取品牌 Logo 配置失败')
  },
)

onMounted(() => {
  setAdminRouteClass(true)
})

onUnmounted(() => {
  setAdminRouteClass(false)
  primeVue.config.locale = getActiveUserPrimeLocale()
})
</script>
