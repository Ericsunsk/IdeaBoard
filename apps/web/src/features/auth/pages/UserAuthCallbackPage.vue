<template>
  <main class="grid min-h-[60vh] place-items-center px-6 py-12 text-ib-text">
    <div class="flex max-w-md flex-col items-center gap-4 text-center" aria-live="polite">
      <ProgressSpinner v-if="status === 'loading'" class="size-8" stroke-width="4" />
      <h1 class="m-0 text-xl font-semibold">{{ title }}</h1>
      <p class="m-0 text-sm leading-6 text-ib-text-muted">{{ message }}</p>
      <Button
        v-if="status === 'failed'"
        label="返回登录"
        severity="secondary"
        outlined
        @click="returnToLogin"
      />
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import ProgressSpinner from 'primevue/progressspinner'
import { useUserStore } from '../session/authSession'
import { USER_LOGIN_PATH } from '../../../shared/constants/routePaths'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const status = ref<'loading' | 'failed'>('loading')
const title = computed(() => (status.value === 'loading' ? '正在完成登录' : '登录未完成'))
const message = computed(() =>
  status.value === 'loading' ? '正在恢复你的 Session...' : 'OAuth 登录失败，请重新尝试。',
)

async function returnToLogin() {
  await router.replace(USER_LOGIN_PATH)
}

onMounted(async () => {
  const providerError = Array.isArray(route.query.error) ? route.query.error[0] : route.query.error
  const succeeded = await userStore.completeOAuthCallback({
    providerError,
    redirectPath: route.query.redirect,
    replace: (target) => router.replace(target),
  })
  if (!succeeded) {
    status.value = 'failed'
  }
})
</script>
