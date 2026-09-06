<template>
  <main class="grid min-h-[60vh] place-items-center px-6 py-12 text-ib-text">
    <div class="flex max-w-md flex-col items-center gap-4 text-center" aria-live="polite">
      <ProgressSpinner v-if="status === 'loading'" class="size-8" stroke-width="4" />
      <h1 class="m-0 text-xl font-semibold">{{ title }}</h1>
      <p class="m-0 text-sm leading-6 text-ib-text-muted">{{ message }}</p>
      <Button
        v-if="status === 'failed'"
        :label="t('auth.backToSignIn')"
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
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import ProgressSpinner from 'primevue/progressspinner'
import { useUserStore } from '../session/authSession'
import { USER_LOGIN_PATH } from '../../../shared/constants/routePaths'
import { isSafeInternalRedirectPath } from '../../../shared/utils/auth'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { t } = useI18n()
const status = ref<'loading' | 'failed'>('loading')
const rawRedirect = Array.isArray(route.query.redirect)
  ? route.query.redirect[0]
  : route.query.redirect
const safeRedirect =
  typeof rawRedirect === 'string' && isSafeInternalRedirectPath(rawRedirect)
    ? rawRedirect
    : undefined
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
  const providerError = Array.isArray(route.query.error) ? route.query.error[0] : route.query.error
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
