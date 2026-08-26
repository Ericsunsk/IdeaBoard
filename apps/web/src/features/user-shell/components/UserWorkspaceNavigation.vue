<template>
  <nav
    class="absolute top-0 left-1/2 flex h-[var(--ib-page-chrome-control-size,40px)] w-max -translate-x-1/2 items-center rounded-full border border-board-card-border bg-board-control-bg p-1 backdrop-blur-md shadow-[var(--shadow-ib-control)] max-[860px]:static max-[860px]:order-3 max-[860px]:mt-4 max-[860px]:w-full max-[860px]:translate-x-0"
    :aria-label="t('common.navigation.workspaceNavigation')"
  >
    <RouterLink
      v-for="item in navigationItems"
      :key="item.path"
      v-slot="{ href, isActive, navigate }"
      :to="item.path"
      custom
    >
      <a
        :href="href"
        :class="[
          navigationItemClass,
          isActive
            ? 'bg-ib-surface-hover text-board-page-text'
            : 'text-board-page-text-muted hover:text-board-page-text',
          item.path === USER_BOARDS_PATH && isWorkspaceNavigationPending
            ? 'cursor-wait'
            : 'cursor-pointer',
        ]"
        :aria-busy="item.path === USER_BOARDS_PATH && isWorkspaceNavigationPending"
        :aria-disabled="
          item.path === USER_BOARDS_PATH && isWorkspaceNavigationPending ? 'true' : undefined
        "
        @click="handleNavigationClick($event, item.path, navigate)"
      >
        <component
          :is="item.icon"
          :class="['size-4 shrink-0', isActive ? 'text-board-primary' : 'text-current']"
          aria-hidden="true"
        />
        <span>{{ item.label }}</span>
      </a>
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Compass, ViewGrid } from '@iconoir/vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { USER_BOARDS_PATH, USER_SEARCH_PATH } from '../../../shared/constants/routePaths'
import { useUserStore } from '../../auth/session/authSession'
import { useAuthOverlay } from '../../auth/authOverlay'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const userStore = useUserStore()
const authOverlay = useAuthOverlay()
const isWorkspaceNavigationPending = ref(false)
const isWorkspaceActive = computed(() => route.path === USER_BOARDS_PATH)
const navigationItemClass =
  'flex h-full flex-none items-center justify-center gap-1.5 whitespace-nowrap rounded-full border border-transparent px-3 text-[13px] leading-none font-medium no-underline transition-[background-color,color] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] max-[860px]:flex-1'

const navigationItems = computed(() => [
  { path: USER_SEARCH_PATH, label: t('common.navigation.search'), icon: Compass },
  { path: USER_BOARDS_PATH, label: t('common.navigation.workspace'), icon: ViewGrid },
] as const)

function handleNavClick(path: string) {
  if (route.path === path && typeof window !== 'undefined') {
    window.scrollTo({ top: 0, behavior: 'smooth' })
    const pageEl = document.querySelector('.app-scrollbar')
    if (pageEl) pageEl.scrollTo({ top: 0, behavior: 'smooth' })
  }
}

function handleNavigationClick(
  event: MouseEvent,
  path: string,
  navigate: (event?: MouseEvent) => unknown,
) {
  if (path === USER_BOARDS_PATH) {
    event.preventDefault()
    void handleGatedWorkspaceClick()
    return
  }

  handleNavClick(path)
  void navigate(event)
}

async function handleGatedWorkspaceClick() {
  if (isWorkspaceNavigationPending.value) return

  isWorkspaceNavigationPending.value = true
  try {
    const user = await userStore.ensureSession({ fresh: true }).catch(() => null)
    if (user) {
      handleNavClick(USER_BOARDS_PATH)
      if (!isWorkspaceActive.value) await router.push(USER_BOARDS_PATH)
      return
    }

    // A local user cache is not authorization. Only resume navigation after the session seam succeeds.
    // This keeps a stale session on the current public surface instead of letting the route guard redirect.
    authOverlay.commands.open('login', isWorkspaceActive.value ? undefined : USER_BOARDS_PATH)
  } finally {
    isWorkspaceNavigationPending.value = false
  }
}
</script>
