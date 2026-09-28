<template>
  <div class="flex h-screen w-full flex-col overflow-hidden">
    <div
      v-if="isImpersonating"
      class="fixed top-2.5 left-1/2 z-[1200] flex min-h-10 w-max max-w-[calc(100vw-20px)] -translate-x-1/2 items-center justify-between gap-2.5 rounded-ib-panel border border-ib-creative-accent/25 bg-ib-creative-accent-soft/90 px-2.5 py-1.5 text-ib-text backdrop-blur-[10px] top-2 max-w-[calc(100vw-12px)] rounded-ib-control px-2 py-1 md:top-2.5 md:max-w-[calc(100vw-20px)] md:rounded-ib-panel md:px-2.5 md:py-1.5"
    >
      <div class="inline-flex min-w-0 items-center gap-2">
        <span
          class="inline-flex size-[22px] shrink-0 items-center justify-center rounded-ib-control bg-ib-creative-accent/15 text-ib-creative-accent-pressed"
        >
          <Shield class="size-3.5" />
        </span>
        <span class="whitespace-nowrap text-xs font-semibold">{{
          t('common.impersonation.activeBanner')
        }}</span>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        class="size-[26px] min-w-[26px] shrink-0 rounded-full border border-ib-creative-accent/35 bg-ib-surface p-0 text-ib-creative-accent-pressed shadow-none hover:border-ib-creative-accent/50 hover:bg-ib-creative-accent-soft active:border-ib-creative-accent-pressed/55 active:bg-ib-creative-accent/15 focus-visible:outline-none focus-visible:shadow-ib-focus disabled:cursor-wait disabled:opacity-70 size-6 min-w-6 md:size-[26px] md:min-w-[26px]"
        :aria-label="t('common.impersonation.exit')"
        :aria-busy="stopImpersonatingLoading"
        :disabled="stopImpersonatingLoading"
        @click="handleStopImpersonating"
      >
        <Xmark class="size-[13px] md:size-3.5" aria-hidden="true" />
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
import { Button } from '@/components/ui/button'
import {
  UserAuthModals,
  createAuthOverlay,
  provideAuthOverlay,
  useAuthFeedback,
  useAuthSession,
  useUserStore,
} from '../../auth'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import {
  ADMIN_USERS_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  USER_CANVAS_PATH,
} from '../../../shared/constants/routePaths'
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
