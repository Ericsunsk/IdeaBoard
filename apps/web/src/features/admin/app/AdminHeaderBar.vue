<template>
  <div class="sticky top-0 z-admin-header flex shrink-0 flex-col bg-background">
    <header
      class="flex h-14 shrink-0 items-center justify-between border-b border-border px-4 sm:px-6 lg:px-8"
    >
      <div class="flex min-w-0 items-center gap-3">
        <button
          type="button"
          aria-label="切换导航菜单"
          class="flex size-9 shrink-0 items-center justify-center rounded-control border border-border text-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted lg:hidden"
          @click="drawerOpen = !drawerOpen"
        >
          <MenuIcon v-if="!drawerOpen" class="size-5" />
          <Xmark v-else class="size-5" />
        </button>

        <button
          type="button"
          title="回到顶部"
          class="hidden min-h-8 min-w-0 items-center gap-2 text-left transition duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] lg:flex"
          :class="
            stickyTitleVisible
              ? 'translate-y-0 opacity-100'
              : 'pointer-events-none -translate-y-1 opacity-0'
          "
          @click="$emit('scrollToTop')"
        >
          <component
            :is="activeItem.icon"
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <span class="truncate text-base font-semibold text-foreground">
            {{ dynamicTitle || currentMeta.title }}
          </span>
        </button>
      </div>

      <div class="flex shrink-0 items-center gap-2">
        <AdminPopover>
          <AdminPopoverTrigger as-child>
            <button
              title="异常雷达"
              type="button"
              :class="[notificationButtonClass, 'cursor-pointer']"
              :aria-label="hasAnomalies ? `异常雷达，${anomalyCount} 项活跃异常` : '异常雷达'"
              aria-haspopup="dialog"
            >
              <BellIcon class="size-4" />
              <span
                v-if="hasAnomalies"
                class="absolute top-[7px] right-[7px] size-2.5 rounded-pill bg-destructive ring-2 ring-card"
                aria-hidden="true"
              ></span>
            </button>
          </AdminPopoverTrigger>
          <AdminPopoverContent
            :side-offset="8"
            class="w-admin-popover max-w-[calc(100vw_-_28px)] p-0 shadow-admin-popover"
          >
            <div :class="adminAnomalyPopoverClass">
              <div
                class="flex items-start justify-between gap-4 border-b border-border px-4 py-3.5"
              >
                <div class="min-w-0">
                  <div class="text-admin-content font-bold text-foreground">异常雷达</div>
                </div>
                <AdminStatusPill
                  :label="hasAnomalies ? `${anomalyCount} 项` : '正常'"
                  :variant="hasAnomalies ? 'danger' : 'success'"
                  :pulse="hasAnomalies"
                  size="small"
                />
              </div>

              <div v-if="hasAnomalies" class="app-scrollbar min-h-0 flex-1 overflow-y-auto p-2">
                <button
                  v-for="item in anomalies"
                  :key="item.id"
                  type="button"
                  class="flex w-full min-w-0 cursor-pointer gap-3 rounded-panel px-3 py-3 text-left transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted"
                  @click="handleAnomalySelect(item)"
                >
                  <span
                    class="mt-[7px] size-2 shrink-0 rounded-pill bg-destructive"
                    aria-hidden="true"
                  ></span>
                  <span class="min-w-0 flex-1">
                    <span class="flex min-w-0 items-center justify-between gap-3">
                      <span class="truncate text-admin-body font-bold text-foreground">
                        {{ item.title }}
                      </span>
                      <span class="shrink-0 text-admin-caption font-bold text-muted-foreground">
                        {{ item.metric || item.actionLabel }}
                      </span>
                    </span>
                    <span class="mt-1 block text-admin-meta leading-5 text-muted-foreground">
                      {{ item.summary }}
                    </span>
                    <span class="mt-1.5 block text-admin-caption font-bold text-muted-foreground">
                      {{ item.actionLabel }}
                    </span>
                  </span>
                </button>
              </div>

              <div v-else class="px-5 py-8 text-center">
                <div class="text-admin-content font-bold text-foreground">运行正常</div>
                <div class="mt-1.5 text-admin-meta text-muted-foreground">当前没有活跃异常</div>
              </div>
            </div>
          </AdminPopoverContent>
        </AdminPopover>

        <UserAccountMenu
          ref="userAccountMenuRef"
          v-model:open="userMenuOpen"
          :avatar-url="userStore.user?.avatar || undefined"
          :user-email="userEmail"
          :items="accountMenuItems"
          :support-channels="supportChannels"
          :workspace="workspaceSwitcher.menuProps.value"
          :avatar-button-class="userAvatarButtonClass"
          :avatar-image-class="userAvatarImageClass"
          :avatar-placeholder-class="userAvatarPlaceholderClass"
          :avatar-icon-class="userAvatarIconClass"
        />
      </div>
    </header>
    <!-- 二级子导航栏（插槽/Teleport 挂载区：有子页面时挂载，无子页面时 empty:hidden 高度折叠） -->
    <div id="admin-sub-header" class="border-b border-border empty:hidden"></div>
  </div>
</template>

<script setup lang="ts">
import { useSupportChannels } from '../../../core/composables/useSupportChannels'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AdminPopover } from '../shared'
import { AdminPopoverTrigger } from '../shared'
import { AdminPopoverContent } from '../shared'
import { AdminStatusPill } from '../shared'
import { BellNotificationSolid as BellIcon, Menu as MenuIcon, Xmark } from '@iconoir/vue'
import type { AdminNavigationItem } from './navigation'
import { useUserStore } from '../../auth'
import { useLogoutRedirect } from '../../../core/composables/useLogoutRedirect'
import { useAnomalyRadar } from '../shared'
import { useAdminToast } from '../shared'
import type { AdminAnomalyItem } from '../shared'
import { adminAnomalyPopoverClass } from '../shared'
import { USER_BOARDS_PATH, USER_LOGIN_PATH } from '../../workspace/routePaths'
import { useWorkspaceSwitcher } from '../../workspace'
import UserAccountMenu from '../../account/ui/chrome/UserAccountMenu.vue'
import type { UserAccountMenuItem } from '../../account/ui/chrome/userAccountMenu.types'

defineProps<{
  activeItem: AdminNavigationItem
  currentMeta: { title: string; subtitle?: string | undefined }
  stickyTitleVisible: boolean
  dynamicTitle: string
}>()

defineEmits<{
  (e: 'scrollToTop'): void
}>()

const drawerOpen = defineModel<boolean>('drawerOpen', { required: true })

const toast = useAdminToast()
const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })

const userMenuOpen = ref(false)
const { channels: supportChannels } = useSupportChannels()
const userAccountMenuRef = ref<InstanceType<typeof UserAccountMenu> | null>(null)
const workspaceSwitcher = useWorkspaceSwitcher()

const userEmail = computed(() => {
  return userStore.email || '--'
})

const accountMenuItems: UserAccountMenuItem[] = [
  { key: 'home', label: '返回画布', command: () => handleUserAction('boards') },
  { key: 'logout', label: '退出登录', tone: 'danger', command: () => handleUserAction('logout') },
]

const { anomalies, anomalyCount, hasAnomalies } = useAnomalyRadar({
  enabled: computed(() => true),
})

const notificationButtonClass =
  'relative flex size-9 items-center justify-center rounded-control text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted'
const userAvatarButtonClass =
  'relative flex size-9 items-center justify-center rounded-control text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted cursor-pointer'
const userAvatarImageClass = 'size-6 rounded-pill object-cover'
const userAvatarPlaceholderClass =
  'flex size-full items-center justify-center text-muted-foreground'
const userAvatarIconClass = 'size-4'

function handleAnomalySelect(item: AdminAnomalyItem) {
  if (route.path === item.route) {
    toast.info(`已在当前页面：${item.title}`)
    return
  }
  void router.push(item.route)
}

function handleUserAction(action: 'boards' | 'logout') {
  userMenuOpen.value = false
  if (action === 'boards') {
    void router.push(USER_BOARDS_PATH)
    return
  }
  void logoutAndRedirect(USER_LOGIN_PATH, { replace: true })
}

defineExpose({
  closeUserMenu: () => {
    userAccountMenuRef.value?.closeMenu()
  },
})
</script>
