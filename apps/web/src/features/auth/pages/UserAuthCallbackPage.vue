<template>
  <main class="grid min-h-[60vh] place-items-center px-6 py-12 text-ib-text">
    <div class="flex max-w-md flex-col items-center gap-4 text-center" aria-live="polite">
      <Spinner v-if="status === 'loading'" class="size-8" />
      <h1 class="m-0 text-xl font-semibold">{{ title }}</h1>
      <p class="m-0 text-sm leading-6 text-ib-text-muted">{{ message }}</p>
      <Button v-if="status === 'failed'" variant="outline" @click="returnToLogin">
        {{ t('auth.backToSignIn') }}
      </Button>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useUserStore } from '../session/userSession'
import { USER_LOGIN_PATH } from '../../../shared/constants/routePaths'
import { isSafeInternalRedirectPath } from '../../../shared/utils/auth'
import { stringParam, stringParamWhere } from '../../../shared/router/queryParams'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { t } = useI18n()
const status = ref<'loading' | 'failed'>('loading')
const safeRedirect = stringParamWhere(route.query.redirect, isSafeInternalRedirectPath)
const title = computed(() =>
  status.value === 'loading' ? t('auth.oauth.completingTitle') : t('auth.oauth.failedTitle'),
)
const message = computed(() =>
  status.value === 'loading' ? t('auth.oauth.completingMessage') : t('auth.oauth.failedMessage'),
)

async function returnToLogin() {
  await router.replace(
    safeRedirect ? { path: USER_LOGIN_PATH, query: { redirect: safeRedirect } } : USER_LOGIN_PATH,
  )
}

onMounted(async () => {
  const providerError = stringParam(route.query.error)
  const succeeded = await userStore.completeOAuthCallback({
    providerError,
    redirectPath: safeRedirect,
    replace: (target) => router.replace(target),
  })
  if (!succeeded) {
    status.value = 'failed'
  }
})
</script>
