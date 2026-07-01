<template>
  <div class="flex h-screen w-full flex-col overflow-hidden">
    <div
      v-if="isImpersonating"
      class="fixed top-2.5 left-1/2 z-[1200] flex min-h-10 w-max max-w-[calc(100vw-20px)] -translate-x-1/2 items-center justify-between gap-2.5 rounded-[14px] border border-primary-300/25 bg-primary-50/90 px-2.5 py-1.5 text-surface-800 backdrop-blur-[10px] max-[720px]:top-2 max-[720px]:max-w-[calc(100vw-12px)] max-[720px]:rounded-xl max-[720px]:px-2 max-[720px]:py-1"
    >
      <div class="inline-flex min-w-0 items-center gap-2">
        <span
          class="inline-flex size-[22px] shrink-0 items-center justify-center rounded-[7px] bg-primary-200/40 text-primary-700"
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
        class="!size-[26px] !min-w-[26px] !shrink-0 !border !border-primary-300/35 !bg-primary-50 !p-0 !text-primary-700 !shadow-none hover:!border-primary-400/50 hover:!bg-primary-100 active:!border-primary-500/55 active:!bg-primary-200/60 disabled:!cursor-wait disabled:!opacity-70 max-[720px]:!size-6 max-[720px]:!min-w-6"
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

    <UserAuthModals v-if="authMode" :mode="authMode" />
  </div>
</template>

<script setup lang="ts">
import { Shield, Xmark } from '@iconoir/vue'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { useUserStore } from '../../../app/stores/user'
import { authClient, extractBetterAuthErrorMessage } from '../../../shared/auth/client'
import { useAppToast } from '../../../shared/composables/useAppToast'
import {
  ADMIN_USERS_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  USER_CANVAS_PATH,
} from '../../../shared/constants/routePaths'
import UserAuthModals from '../../auth/components/UserAuthModals.vue'

const router = useRouter()
const route = useRoute()
const appToast = useAppToast()
const userStore = useUserStore()

const stopImpersonatingLoading = ref(false)
const authSession = authClient.useSession()
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
  stopImpersonatingLoading.value = true
  try {
    const { error } = await authClient.admin.stopImpersonating()
    if (error) throw error
    await userStore.fetchUserInfo(true)
    await authSession.value?.refetch?.()
    appToast.success('已退出伪装身份')
    await router.push(ADMIN_USERS_PATH)
  } catch (err: unknown) {
    appToast.error(extractBetterAuthErrorMessage(err, '退出伪装失败，请重新登录管理员账号'))
  } finally {
    stopImpersonatingLoading.value = false
  }
}
</script>
