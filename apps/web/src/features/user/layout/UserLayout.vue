<template>
  <div class="flex h-screen w-full flex-col overflow-hidden">
    <div
      v-if="isImpersonating"
      class="fixed left-1/2 top-2.5 z-[1200] flex min-h-10 w-max max-w-[calc(100vw-20px)] -translate-x-1/2 items-center justify-between gap-2.5 rounded-[14px] border border-[#7a6bd5]/25 bg-[#f6f7ff]/90 px-2.5 py-1.5 text-[#2f3750] backdrop-blur-[10px] max-[720px]:top-2 max-[720px]:max-w-[calc(100vw-12px)] max-[720px]:rounded-xl max-[720px]:px-2 max-[720px]:py-1"
    >
      <div class="inline-flex min-w-0 items-center gap-2">
        <span
          class="inline-flex size-[22px] shrink-0 items-center justify-center rounded-[7px] bg-[#7a6bd5]/15 text-[#5d4ec3]"
        >
          <Shield class="size-3.5" />
        </span>
        <span class="whitespace-nowrap text-xs font-semibold text-[#2f3750]">伪装预览中</span>
      </div>
      <Button
        type="button"
        rounded
        text
        class="!size-[26px] !min-w-[26px] !shrink-0 !border !border-[#7a6bd5]/35 !bg-[#7a6bd5]/5 !p-0 !text-[#5d4ec3] !shadow-none hover:!border-[#7a6bd5]/50 hover:!bg-[#7a6bd5]/10 hover:!text-[#5244b4] active:!border-[#7a6bd5]/55 active:!bg-[#7a6bd5]/15 active:!text-[#4a3ca7] disabled:!cursor-wait disabled:!opacity-70 max-[720px]:!size-6 max-[720px]:!min-w-6"
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
      <router-view :key="route.path === '/user/picture' ? route.path : route.fullPath" />
    </div>

    <UserAuthModals v-if="authMode" :mode="authMode" />
  </div>
</template>

<script setup lang="ts">
import { Shield, Xmark } from '@iconoir/vue'
import Button from 'primevue/button'
import { useUserStore } from '../../../app/stores/user'
import { authClient, extractBetterAuthErrorMessage } from '../../../shared/auth/client'
import { useAppToast } from '../../../shared/composables/useAppToast'
import UserAuthModals from '../../auth/components/UserAuthModals.vue'

const router = useRouter()
const route = useRoute()
const appToast = useAppToast()
const userStore = useUserStore()

const stopImpersonatingLoading = ref(false)
const authSession = authClient.useSession()
const isImpersonating = computed(() =>
  Boolean((authSession.value?.data as any)?.session?.impersonatedBy),
)
const authMode = computed<'login' | 'register' | null>(() => {
  if (route.path === '/user/login') return 'login'
  if (route.path === '/user/register') return 'register'
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
    router.push('/admin/users')
  } catch (err: any) {
    appToast.error(extractBetterAuthErrorMessage(err, '退出伪装失败，请重新登录管理员账号'))
  } finally {
    stopImpersonatingLoading.value = false
  }
}
</script>
