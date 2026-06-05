<template>
  <router-view v-if="isLoginRoute" />
  <div v-else :class="layoutShellClass">
    <aside :class="siderClass" aria-label="后台导航">
      <router-link :class="brandMarkClass" :to="ADMIN_DEFAULT_PATH" aria-label="IdeaBoard Admin">
        <span :class="brandTextClass">IB</span>
      </router-link>

      <nav :class="navRailClass" aria-label="后台功能">
        <Button
          v-for="item in adminNavigationItems"
          :key="item.key"
          v-tooltip.right="item.label"
          type="button"
          :aria-label="item.label"
          :aria-current="activeKey === item.key ? 'page' : undefined"
          text
          rounded
          :class="[
            railButtonClass,
            activeKey === item.key ? railButtonActiveClass : railButtonIdleClass,
          ]"
          @click="handleMenuUpdate(item.key)"
        >
          <template #icon>
            <component :is="item.icon" :class="navIconClass" />
          </template>
        </Button>
      </nav>

      <div :class="siderBottomClass">
        <Button
          v-tooltip.right="'个人中心'"
          type="button"
          text
          rounded
          :class="[railButtonClass, railButtonIdleClass, '!overflow-hidden !rounded-full']"
          aria-haspopup="menu"
          :aria-expanded="false"
          @click="toggleUserMenu"
        >
          <template #icon>
            <Avatar
              v-if="userStore.user?.avatar"
              :image="userStore.user.avatar"
              shape="circle"
              :class="avatarClass"
            />
            <Avatar
              v-else
              icon="pi pi-user"
              shape="circle"
              :class="[avatarClass, '!border !border-[var(--color-admin-border)]']"
            />
          </template>
        </Button>
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
            <p class="m-0 mb-1 text-[13px] font-bold tracking-normal text-[#6f7c91] max-[1080px]:hidden">
              Overview
            </p>
            <h1
              class="m-0 text-[26px] leading-[1.05] font-[780] tracking-normal text-[#2d374c] max-[1080px]:text-2xl"
            >
              {{ currentMeta.title }}
            </h1>
          </div>
        </div>

        <div class="flex min-w-0 items-center gap-3.5">
          <IconField class="w-[min(300px,30vw)] max-[720px]:hidden">
            <InputIcon class="!text-[#8795aa]">
              <SearchIcon class="size-4 stroke-2" />
            </InputIcon>
            <InputText :class="searchInputClass" placeholder="Search" aria-label="Search" />
          </IconField>

          <Button
            v-tooltip.bottom="'通知'"
            :class="notificationButtonClass"
            type="button"
            text
            rounded
            aria-label="通知"
          >
            <template #icon>
              <BellIcon :class="shellIconClass" />
            </template>
            <span
              class="absolute top-[7px] right-[7px] size-2 rounded-full border-2 border-white bg-[#e45656]"
              aria-hidden="true"
            ></span>
          </Button>
        </div>
      </header>

      <section :class="contentClass">
        <router-view />
      </section>
    </main>

    <Menu ref="userMenuRef" :model="userMenuItems" popup :pt="userMenuPt" />
    <ConfirmDialog />

    <Drawer
      v-model:visible="mobileMenuOpen"
      position="left"
      :pt="mobileDrawerPt"
      :show-close-icon="false"
    >
      <template #container="{ closeCallback }">
        <div class="flex h-full flex-col gap-6 bg-[#f6f8fc] p-[22px]">
          <div class="flex items-center justify-between">
            <router-link
              :class="[brandMarkClass, '!ml-0']"
              :to="ADMIN_DEFAULT_PATH"
              aria-label="IdeaBoard Admin"
              @click="closeCallback"
            >
              <span :class="brandTextClass">IB</span>
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
            <Button
              v-for="item in adminNavigationItems"
              :key="item.key"
              type="button"
              text
              :label="item.label"
              :class="[
                mobileNavButtonClass,
                activeKey === item.key ? mobileNavButtonActiveClass : mobileNavButtonIdleClass,
              ]"
              @click="handleMobileMenuSelect(item.key, closeCallback)"
            >
              <template #icon>
                <component :is="item.icon" :class="navIconClass" />
              </template>
            </Button>
          </nav>
        </div>
      </template>
    </Drawer>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import ConfirmDialog from 'primevue/confirmdialog'
import Drawer from 'primevue/drawer'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import Tooltip from 'primevue/tooltip'
import type { MenuItem } from 'primevue/menuitem'
import type { AdminNavigationKey } from './adminNavigation'
import { Bell as BellIcon, Menu as MenuIcon, Search as SearchIcon, Xmark } from '@iconoir/vue'
import {
  adminNavigationByKey,
  adminNavigationItems,
  ADMIN_DEFAULT_PATH,
  ADMIN_LOGIN_PATH,
  findAdminNavigationItemByPath,
} from './adminNavigation'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'

const vTooltip = Tooltip

type PrimePopupMenu = {
  toggle: (event: Event) => void
}

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect()

const activeKey = ref<AdminNavigationKey>('users')
const mobileMenuOpen = ref(false)
const userMenuRef = ref<PrimePopupMenu | null>(null)
const isLoginRoute = computed(() => route.path === ADMIN_LOGIN_PATH)

const layoutShellClass =
  'grid min-h-screen grid-cols-[133px_minmax(0,1fr)] bg-white text-[var(--color-admin-text-1)] max-[1080px]:grid-cols-1'
const siderClass =
  "sticky top-0 flex h-screen self-start flex-col items-start bg-white py-[42px] max-[1080px]:hidden before:absolute before:top-[21px] before:bottom-[21px] before:left-[21px] before:box-border before:w-[84px] before:rounded-[14px] before:border before:border-[#f1f5f9] before:bg-[#f8fafc] before:content-['']"
const brandMarkClass =
  'relative z-[1] ml-[42px] inline-flex size-10 flex-none items-center justify-center rounded-xl border-0 bg-white text-[#0f172a] no-underline shadow-none focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_var(--color-admin-focus-ring)]'
const brandTextClass = 'text-xs font-[850] tracking-normal'
const navRailClass = 'relative z-[1] ml-[21px] flex w-[84px] flex-1 flex-col items-center gap-3 pt-3.5'
const siderBottomClass = 'relative z-[1] ml-[21px] flex w-[84px] flex-col items-center gap-3'
const focusRingClass =
  'focus-visible:!outline-none focus-visible:!shadow-[0_0_0_3px_var(--color-admin-focus-ring)]'
const railButtonClass = [
  '!inline-flex !size-10 !items-center !justify-center !rounded-[10px] !border !border-transparent !bg-transparent !text-[#718096] !shadow-none !transition-colors !duration-200',
  focusRingClass,
].join(' ')
const railButtonIdleClass =
  'hover:!border-[#d7e0eb] hover:!bg-[#eef2f7] hover:!text-[var(--color-admin-text-1)]'
const railButtonActiveClass =
  '!border-[#040819] !bg-[#05091a] !text-white !shadow-[0_8px_18px_rgba(5,9,26,0.16)]'
const shellButtonSkinClass = [
  '!size-[42px] !items-center !justify-center !rounded-[9px] !border !border-transparent !bg-transparent !text-[#718096] !shadow-none !transition-colors !duration-200',
  'hover:!border-[#d7e0eb] hover:!bg-[#eef2f7] hover:!text-[var(--color-admin-text-1)]',
  focusRingClass,
].join(' ')
const shellButtonClass = ['!inline-flex', shellButtonSkinClass].join(' ')
const mobileMenuButtonClass = [
  '!hidden max-[1080px]:!inline-flex',
  shellButtonSkinClass,
  '!border-[var(--color-admin-border)] !bg-white',
].join(' ')
const notificationButtonClass = [
  shellButtonClass,
  'relative !rounded-lg !border-[#d6dee9] !bg-white !shadow-[0_3px_10px_rgba(45,64,89,0.05)]',
].join(' ')
const navIconClass = 'size-[17px] stroke-2'
const shellIconClass = 'size-[19px] stroke-2'
const avatarClass = '!size-[30px] !bg-white !text-[var(--color-admin-text-1)]'
const mainClass = 'flex min-h-screen min-w-0 flex-col overflow-visible bg-white'
const headerClass =
  'flex min-h-[88px] items-start justify-between gap-6 bg-white py-[21px] pr-[42px] pl-[7px] max-[1080px]:min-h-[92px] max-[1080px]:items-center max-[1080px]:px-[22px] max-[1080px]:pt-[22px] max-[1080px]:pb-4 max-[720px]:gap-3 max-[720px]:px-[14px] max-[720px]:pt-4 max-[720px]:pb-3'
const searchInputClass =
  '!h-[38px] !w-full !rounded-lg !border-[#d6dee9] !bg-white !text-[15px] !text-[var(--color-admin-text-1)] !shadow-[0_3px_10px_rgba(45,64,89,0.05)] placeholder:!text-[#7a8798]'
const contentClass =
  'min-h-0 flex-1 overflow-visible bg-white pr-[42px] pb-[38px] max-[1080px]:px-[22px] max-[1080px]:pb-7 max-[720px]:px-[14px] max-[720px]:pb-5'
const mobileNavButtonClass = [
  '!min-h-12 !w-full !justify-start !gap-3 !rounded-xl !border !border-transparent !bg-transparent !text-sm !font-bold !text-[var(--color-admin-text-2)]',
  focusRingClass,
].join(' ')
const mobileNavButtonIdleClass =
  'hover:!border-[var(--color-admin-border)] hover:!bg-white hover:!text-[var(--color-admin-text-1)]'
const mobileNavButtonActiveClass = '!bg-[#05091a] !text-white'
const userMenuPt = {
  root: {
    class:
      'min-w-[180px] overflow-hidden rounded-[14px] border border-admin-border bg-white shadow-[0_18px_42px_rgba(45,64,89,0.14)]',
  },
  list: { class: '!m-0 !p-1.5' },
  itemContent: { class: 'rounded-[10px] hover:!bg-admin-surface-hover' },
  itemLink: {
    class:
      '!px-3 !py-2.5 !text-admin-body !font-[650] !text-admin-text-2 hover:!text-admin-text-1',
  },
} as const
const mobileDrawerPt = {
  root: {
    class: '!w-[min(320px,calc(100vw-32px))] !border-0 !bg-[#f6f8fc]',
  },
} as const

watch(
  () => route.path,
  (path) => {
    const item = findAdminNavigationItemByPath(path)
    if (item) {
      activeKey.value = item.key
    }
    mobileMenuOpen.value = false
  },
  { immediate: true },
)

const currentMeta = computed(() => {
  return adminNavigationByKey[activeKey.value] || { title: '管理后台' }
})

const userMenuItems = computed<MenuItem[]>(() => [
  {
    label: userStore.user?.username || 'Admin',
    disabled: true,
  },
  { separator: true },
  {
    label: '个人中心',
    command: () => {
      void handleUserAction('profile')
    },
  },
  {
    label: '退出登录',
    command: () => {
      void handleUserAction('logout')
    },
  },
])

function handleMenuUpdate(key: AdminNavigationKey) {
  const item = adminNavigationByKey[key]
  if (item) {
    activeKey.value = item.key
    void router.push(item.path)
  }
}

function handleMobileMenuSelect(key: AdminNavigationKey, closeCallback: () => void) {
  handleMenuUpdate(key)
  closeCallback()
}

function toggleUserMenu(event: Event) {
  userMenuRef.value?.toggle(event)
}

async function handleUserAction(key: 'profile' | 'logout') {
  if (key === 'logout') {
    await logoutAndRedirect('/admin/login')
  } else {
    await router.push('/user/center')
  }
}

function setAdminRouteClass(enabled: boolean) {
  document.body.classList.toggle('admin-route', enabled)
}

onMounted(() => {
  setAdminRouteClass(true)
})

onUnmounted(() => {
  setAdminRouteClass(false)
})
</script>

<style>
body.admin-route .p-tooltip .p-tooltip-text {
  padding: 8px 14px;
  border-radius: 8px;
  background: #2f3b4f;
  color: #ffffff;
  box-shadow: 0 14px 28px rgba(47, 59, 79, 0.22);
  font-size: 14px;
  font-weight: 650;
}

body.admin-route .p-tooltip-right .p-tooltip-arrow {
  border-right-color: #2f3b4f;
}

body.admin-route .p-tooltip-bottom .p-tooltip-arrow {
  border-bottom-color: #2f3b4f;
}
</style>
