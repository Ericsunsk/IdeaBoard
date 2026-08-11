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
        <span class="whitespace-nowrap text-xs font-semibold">伪装预览中</span>
      </div>
      <Button
        type="button"
        rounded
        text
        severity="secondary"
        class="!size-[26px] !min-w-[26px] !shrink-0 !border !border-ib-creative-accent/35 !bg-ib-surface !p-0 !text-ib-creative-accent-pressed !shadow-none hover:!border-ib-creative-accent/50 hover:!bg-ib-creative-accent-soft active:!border-ib-creative-accent-pressed/55 active:!bg-ib-creative-accent/15 focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-wait disabled:!opacity-70 max-[720px]:!size-6 max-[720px]:!min-w-6"
        aria-label="退出伪装身份"
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

    <AccountManagementDialogHost />
    <UserAuthModals v-if="authMode" :mode="authMode" />
  </div>
</template>

<script setup lang="ts">
import { Shield, Xmark } from '@iconoir/vue'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { stopImpersonating, useAuthSession, useUserStore } from '../../auth/session/authSession'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import {
  ADMIN_USERS_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  USER_CANVAS_PATH,
} from '../../../shared/constants/routePaths'
import { useAuthFeedback } from '../../auth/useAuthFeedback'
import UserAuthModals from '../../auth/components/UserAuthModals.vue'
import AccountManagementDialogHost from '../../account-management/components/AccountManagementDialogHost.vue'
import {
  createAccountManagementOverlay,
  provideAccountManagementOverlay,
} from '../../account-management/accountManagementOverlay'

const router = useRouter()
const route = useRoute()
const authFeedback = useAuthFeedback()
const { runAsyncAction } = useAsyncAction()
const userStore = useUserStore()
const accountManagementOverlay = createAccountManagementOverlay({
  currentUser: computed(() => userStore.user),
  reportError: (error) => console.warn('[account-management] payment-return cleanup failed', error),
  navigation: {
    replace: (location) => router.replace(location),
    resolve: (location) => router.resolve(location),
  },
  route,
})
provideAccountManagementOverlay(accountManagementOverlay)

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
const authMode = computed<'login' | 'register' | null>(() => {
  if (route.path === USER_LOGIN_PATH) return 'login'
  if (route.path === USER_REGISTER_PATH) return 'register'
  return null
})

async function handleStopImpersonating() {
  await runAsyncAction(
    async () => {
      await stopImpersonating()
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
