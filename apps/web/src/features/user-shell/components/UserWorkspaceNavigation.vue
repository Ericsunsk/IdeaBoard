<template>
  <nav
    class="absolute top-0 left-1/2 flex h-[var(--ib-page-chrome-control-size,40px)] w-[220px] -translate-x-1/2 items-center rounded-full border border-board-card-border bg-board-control-bg p-1 max-[860px]:static max-[860px]:order-3 max-[860px]:mt-4 max-[860px]:w-full max-[860px]:translate-x-0"
    aria-label="Workspace navigation"
  >
    <template v-for="item in navigationItems" :key="item.path">
      <button
        v-if="item.path === USER_BOARDS_PATH"
        type="button"
        :class="[
          'flex h-full flex-1 items-center justify-center gap-1.5 rounded-full border border-transparent px-3 text-[13px] font-semibold transition-[background-color,color] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] cursor-pointer disabled:cursor-wait',
          isWorkspaceActive
            ? 'bg-ib-surface-hover text-board-page-text'
            : 'text-board-page-text-muted hover:text-board-page-text',
        ]"
        :aria-busy="isWorkspaceNavigationPending"
        :disabled="isWorkspaceNavigationPending"
        @click="handleGatedWorkspaceClick"
      >
        <component
          :is="item.icon"
          :class="['size-4 shrink-0', isWorkspaceActive ? 'text-board-primary' : 'text-current']"
          aria-hidden="true"
        />
        <span>{{ item.label }}</span>
      </button>
      <RouterLink
        v-else
        v-slot="{ isActive }"
        :to="item.path"
        exact-active-class="bg-ib-surface-hover text-board-page-text"
        class="flex h-full flex-1 items-center justify-center gap-1.5 rounded-full border border-transparent px-3 text-[13px] font-semibold text-board-page-text-muted no-underline transition-[background-color,color] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] hover:text-board-page-text focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
        @click="handleNavClick(item.path)"
      >
        <component
          :is="item.icon"
          :class="['size-4 shrink-0', isActive ? 'text-board-primary' : 'text-current']"
          aria-hidden="true"
        />
        <span>{{ item.label }}</span>
      </RouterLink>
    </template>
  </nav>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Compass, ViewGrid } from '@iconoir/vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { USER_BOARDS_PATH, USER_SEARCH_PATH } from '../../../shared/constants/routePaths'
import { useUserStore } from '../../auth/session/authSession'
import { useAuthOverlay } from '../../auth/authOverlay'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const authOverlay = useAuthOverlay()
const isWorkspaceNavigationPending = ref(false)
const isWorkspaceActive = computed(() => route.path === USER_BOARDS_PATH)

const navigationItems = [
  { path: USER_SEARCH_PATH, label: 'Search', icon: Compass },
  { path: USER_BOARDS_PATH, label: 'Workspace', icon: ViewGrid },
] as const

function handleNavClick(path: string) {
  if (route.path === path && typeof window !== 'undefined') {
    window.scrollTo({ top: 0, behavior: 'smooth' })
    const pageEl = document.querySelector('.app-scrollbar')
    if (pageEl) pageEl.scrollTo({ top: 0, behavior: 'smooth' })
  }
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
