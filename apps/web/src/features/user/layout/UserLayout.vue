<template>
  <div class="flex h-screen w-full flex-col overflow-hidden">
    <div
      v-if="isImpersonating"
      class="fixed top-2.5 left-1/2 z-[1200] flex min-h-10 w-max max-w-[calc(100vw-20px)] -translate-x-1/2 items-center justify-between gap-2.5 rounded-[var(--radius-ib-panel)] border border-ib-creative-accent/25 bg-ib-creative-accent-soft/90 px-2.5 py-1.5 text-ib-text backdrop-blur-[10px] max-[720px]:top-2 max-[720px]:max-w-[calc(100vw-12px)] max-[720px]:rounded-[var(--radius-ib-control)] max-[720px]:px-2 max-[720px]:py-1"
    >
      <div class="inline-flex min-w-0 items-center gap-2">
        <span
          class="inline-flex size-[22px] shrink-0 items-center justify-center rounded-[var(--radius-ib-control)] bg-ib-creative-accent/15 text-ib-creative-accent-pressed"
        >
          <Shield class="size-3.5" />
        </span>
        <span class="whitespace-nowrap text-xs font-semibold">{{
          t('common.impersonation.activeBanner')
        }}</span>
      </div>
      <Button
        type="button"
        rounded
        text
        severity="secondary"
        class="!size-[26px] !min-w-[26px] !shrink-0 !border !border-ib-creative-accent/35 !bg-ib-surface !p-0 !text-ib-creative-accent-pressed !shadow-none hover:!border-ib-creative-accent/50 hover:!bg-ib-creative-accent-soft active:!border-ib-creative-accent-pressed/55 active:!bg-ib-creative-accent/15 focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-wait disabled:!opacity-70 max-[720px]:!size-6 max-[720px]:!min-w-6"
        :aria-label="t('common.impersonation.exit')"
        :aria-busy="stopImpersonatingLoading"
        :disabled="stopImpersonatingLoading"
        :loading="stopImpersonatingLoading"
        @click="handleStopImpersonating"
      >
        <template #icon>
          <Xmark class="size-3.5 max-[720px]:size-[13px]" aria-hidden="true" />
        </template>
      </Button>
    </div>
    <div class="min-h-0 flex-1 overflow-hidden bg-transparent">
      <router-view :key="route.path === USER_CANVAS_PATH ? route.path : route.fullPath" />
    </div>

    <AccountDialogHost />
    <UserAuthModals
      v-if="authModal"
      :mode="authModal.mode"
      :presentation="authModal.presentation"
      @overlay-close="handleAuthOverlayClose"
      @overlay-success="handleAuthOverlaySuccess"
      @update:mode="authOverlay.commands.open"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Shield, Xmark } from '@iconoir/vue'
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { useAuthSession, useUserStore } from '../../auth/session/session'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import {
  ADMIN_USERS_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  USER_CANVAS_PATH,
} from '../../../shared/constants/routePaths'
import { useAuthFeedback } from '../../auth/useAuthFeedback'
import UserAuthModals from '../../auth/components/UserAuthModals.vue'
import { createAuthOverlay, provideAuthOverlay } from '../../auth/overlay'
import { clearPendingSearchAuthAction } from '../../search'
import { AccountDialogHost, createAccountOverlay, provideAccountOverlay } from '../../account'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const authFeedback = useAuthFeedback()
const { runAsyncAction } = useAsyncAction()
const userStore = useUserStore()
const accountOverlay = createAccountOverlay({
  currentUser: computed(() => userStore.user),
  reportError: (error) => console.warn('[account] payment-return cleanup failed', error),
  navigation: {
    replace: (location) => router.replace(location),
    resolve: (location) => router.resolve(location),
  },
  route,
})
provideAccountOverlay(accountOverlay)
const authOverlay = createAuthOverlay()
provideAuthOverlay(authOverlay)

const stopImpersonatingLoading = ref(false)
const authSession = useAuthSession()
type AuthSessionData = {
  session?: {
    impersonatedBy?: unknown
  }
}
const isImpersonating = computed(() =>
  Boolean((authSession.value?.data as AuthSessionData | null | undefined)?.session?.impersonatedBy),
)
const authModal = computed<{
  mode: 'login' | 'register'
  presentation: 'overlay' | 'route'
} | null>(() => {
  if (route.path === USER_LOGIN_PATH) return { mode: 'login', presentation: 'route' }
  if (route.path === USER_REGISTER_PATH) return { mode: 'register', presentation: 'route' }
  const overlayMode = authOverlay.viewModel.mode.value
  return overlayMode ? { mode: overlayMode, presentation: 'overlay' } : null
})

function handleAuthOverlayClose() {
  clearPendingSearchAuthAction()
  authOverlay.commands.clearPostAuthPath()
  authOverlay.commands.close()
}

function resumePostAuthNavigation() {
  const path = authOverlay.commands.consumePostAuthPath()
  if (path && path !== route.fullPath) void router.push(path)
}

function handleAuthOverlaySuccess() {
  authOverlay.commands.close()
  resumePostAuthNavigation()
}

watch(
  () => userStore.user,
  (user) => {
    if (user) resumePostAuthNavigation()
  },
  { immediate: true },
)

async function handleStopImpersonating() {
  await runAsyncAction(
    async () => {
      await userStore.stopImpersonating()
      await userStore.fetchUserInfo(true)
      await authSession.value?.refetch?.()
      await router.push(ADMIN_USERS_PATH)
      return true
    },
    {
      loading: stopImpersonatingLoading,
      feedback: authFeedback.stopImpersonating,
    },
  )
}
</script>
