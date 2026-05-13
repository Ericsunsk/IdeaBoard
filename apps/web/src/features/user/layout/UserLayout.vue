<template>
  <div class="user-layout">
    <div v-if="isImpersonating" class="impersonation-banner">
      <div class="impersonation-copy">
        <span class="impersonation-icon-wrap">
          <n-icon size="14">
            <ShieldHalfOutline />
          </n-icon>
        </span>
        <span class="impersonation-title">伪装预览中</span>
      </div>
      <n-button
        size="small"
        circle
        quaternary
        class="impersonation-exit"
        :loading="stopImpersonatingLoading"
        @click="handleStopImpersonating"
      >
        <template #icon>
          <n-icon>
            <CloseOutline />
          </n-icon>
        </template>
      </n-button>
    </div>
    <div class="layout-content">
      <router-view :key="route.path === '/user/picture' ? route.path : route.fullPath" />
    </div>

    <UserAuthScreen v-if="authMode" :mode="authMode" />
  </div>
</template>

<script setup lang="ts">
import { CloseOutline, ShieldHalfOutline } from '@vicons/ionicons5'
import { useUserStore } from '../../../app/stores/user'
import { authClient, extractBetterAuthErrorMessage } from '../../../shared/auth/client'
import UserAuthScreen from '../../auth/screens/UserAuthScreen.vue'

const router = useRouter()
const route = useRoute()
const message = useMessage()
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
    message.success('已退出伪装身份')
    router.push('/admin/users')
  } catch (err: any) {
    message.error(extractBetterAuthErrorMessage(err, '退出伪装失败，请重新登录管理员账号'))
  } finally {
    stopImpersonatingLoading.value = false
  }
}
</script>

<style scoped>
.user-layout {
  width: 100%;
  height: 100vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.impersonation-banner {
  min-height: 40px;
  padding: 6px 10px;
  background: rgba(246, 247, 255, 0.92);
  border: 1px solid rgba(122, 107, 213, 0.24);
  border-radius: 14px;
  color: #2f3750;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  position: fixed;
  top: 10px;
  left: 50%;
  transform: translateX(-50%);
  width: max-content;
  max-width: calc(100vw - 20px);
  box-shadow: none;
  backdrop-filter: blur(10px);
  z-index: 1200;
}

.impersonation-copy {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 9px;
}

.impersonation-icon-wrap {
  width: 22px;
  height: 22px;
  border-radius: 7px;
  background: rgba(122, 107, 213, 0.15);
  color: #5d4ec3;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.impersonation-title {
  font-size: 12px;
  font-weight: 600;
  color: #2f3750;
  white-space: nowrap;
}

.impersonation-exit {
  flex-shrink: 0;
  width: 26px;
  height: 26px;
  min-width: 26px;
  min-height: 26px;
  border-radius: 999px;
  --n-border: 1px solid rgba(122, 107, 213, 0.34);
  --n-border-hover: 1px solid rgba(122, 107, 213, 0.48);
  --n-border-pressed: 1px solid rgba(122, 107, 213, 0.56);
  --n-color: rgba(122, 107, 213, 0.04);
  --n-color-hover: rgba(122, 107, 213, 0.1);
  --n-color-pressed: rgba(122, 107, 213, 0.14);
  --n-padding: 0;
  --n-icon-size: 14px;
  --n-ripple-color: transparent;
}

:deep(.impersonation-exit.n-button) {
  color: #5d4ec3 !important;
  --n-text-color: #5d4ec3 !important;
  --n-text-color-hover: #5244b4 !important;
  --n-text-color-pressed: #4a3ca7 !important;
}

:deep(.impersonation-exit .n-button__content),
:deep(.impersonation-exit .n-button__icon),
:deep(.impersonation-exit .n-icon) {
  color: inherit !important;
}

@media (max-width: 720px) {
  .impersonation-banner {
    top: 8px;
    max-width: calc(100vw - 12px);
    border-radius: 12px;
    padding: 5px 8px;
  }
  .impersonation-exit {
    width: 24px;
    height: 24px;
    min-width: 24px;
    min-height: 24px;
    --n-icon-size: 13px;
  }
}

.layout-content {
  flex: 1;
  min-height: 0;
  background: transparent;
  overflow: hidden;
}
</style>
