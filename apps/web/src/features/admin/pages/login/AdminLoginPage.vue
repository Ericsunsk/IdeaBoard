<template>
  <main class="grid min-h-screen place-items-center bg-background p-4 text-foreground sm:p-6">
    <div :class="loginCardClass">
      <div class="flex justify-center px-6 pt-[26px] pb-2 sm:px-8 sm:pt-8">
        <router-link :class="brandMarkClass" to="/" :aria-label="projectName">
          <SiteBrandLogo
            :src="logoURL"
            :fallback-text="fallbackText"
            :class="brandMarkClasses.logo"
          />
        </router-link>
      </div>

      <div class="px-6 pt-0 pb-6 sm:px-8 sm:pb-7">
        <div class="p-0 text-center">
          <h1 class="m-0 text-admin-modal-title leading-[1.2] font-bold text-foreground">
            {{ ADMIN_LOGIN_MESSAGES.title }}
          </h1>
        </div>

        <div class="pt-6 pb-0">
          <form class="flex flex-col gap-4" novalidate @submit.prevent="handleLogin">
            <label class="flex flex-col gap-2 text-xs font-bold text-muted-foreground">
              <span>{{ ADMIN_LOGIN_MESSAGES.emailLabel }}</span>
              <div class="relative flex items-center">
                <span
                  class="pointer-events-none absolute left-3 flex items-center text-muted-foreground"
                >
                  <User class="h-4 w-4" />
                </span>
                <AdminInput
                  id="admin-login-email"
                  v-model="formValue.email"
                  name="email"
                  type="email"
                  autocomplete="email"
                  class="!min-h-11 pl-9"
                  placeholder="admin@example.com"
                />
              </div>
            </label>

            <label class="flex flex-col gap-2 text-xs font-bold text-muted-foreground">
              <span>{{ ADMIN_LOGIN_MESSAGES.passwordLabel }}</span>
              <div class="relative flex items-center">
                <AdminInput
                  id="admin-login-password"
                  v-model="formValue.password"
                  name="password"
                  :type="showPassword ? 'text' : 'password'"
                  autocomplete="current-password"
                  class="!min-h-11 pr-10"
                />
                <button
                  type="button"
                  class="absolute right-0 flex size-8 items-center justify-center text-muted-foreground hover:text-foreground cursor-pointer transition-colors"
                  :aria-label="
                    showPassword
                      ? ADMIN_LOGIN_MESSAGES.hidePassword
                      : ADMIN_LOGIN_MESSAGES.showPassword
                  "
                  @click="showPassword = !showPassword"
                >
                  <Eye v-if="!showPassword" class="size-4" />
                  <EyeClosed v-else class="size-4" />
                </button>
              </div>
            </label>

            <p v-if="formError" class="m-0 text-admin-body font-semibold text-destructive">
              {{ formError }}
            </p>

            <AdminButton
              type="submit"
              :class="adminActionButtonClass"
              :loading="loading"
              class="w-full !h-11 font-bold shadow-sm"
            >
              {{ loading ? ADMIN_LOGIN_MESSAGES.loggingIn : ADMIN_LOGIN_MESSAGES.enterAdmin }}
            </AdminButton>
          </form>
        </div>

        <div class="flex justify-center pt-[18px] pb-0">
          <AdminButton
            type="button"
            variant="ghost"
            :class="adminTextButtonClass"
            @click="goUserLogin"
          >
            {{ ADMIN_LOGIN_MESSAGES.userLogin }}
          </AdminButton>
        </div>
      </div>
    </div>
  </main>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../auth'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ADMIN_LOGIN_PATH,
  ADMIN_ROOT_PATH,
  USER_LOGIN_PATH,
  isPathUnderRoute,
} from '../../../../shared/constants/routePaths'
import { ADMIN_DEFAULT_PATH } from '../../app/navigationMeta'
import { Eye, EyeClosed, User } from '@iconoir/vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import SiteBrandLogo from '../../../../shared/components/SiteBrandLogo.vue'
import { useLogoutRedirect } from '../../../../shared/composables/useLogoutRedirect'
import { useSiteBranding } from '../../../../shared/composables/useSiteBranding'
import { i18n } from '../../../../shared/i18n'
import { normalizeSafeInternalRedirectPath } from '../../../../shared/utils/auth'
import { stringParam } from '../../../../shared/router/queryParams'
import { brandMarkClasses, createBrandMarkClass } from '../../../../shared/lib/brandMark'
import { adminActionButtonClass, adminTextButtonClass } from '../../shared/lib/uiClasses'
import { useAdminLoginFeedback } from './useAdminLoginFeedback'
import { ADMIN_LOGIN_MESSAGES } from './messages'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const feedback = useAdminLoginFeedback()
const { fallbackText, logoURL, projectName } = useSiteBranding({
  fallback: i18n.global.t('account.branding.loadFailed'),
})
const brandMarkClass = computed(() =>
  createBrandMarkClass(
    brandMarkClasses.sizes.login,
    logoURL.value ? brandMarkClasses.states.image : brandMarkClasses.states.fallbackStrong,
  ),
)
const loginCardClass =
  'admin-login-card w-[min(420px,100%)] overflow-hidden rounded-[18px] border border-border bg-card shadow-[0_24px_70px_rgba(45,64,89,0.12)]'

const loading = ref(false)
const showPassword = ref(false)
const formError = ref('')
const formValue = ref({
  email: '',
  password: '',
})

function resolveAdminRedirectPath() {
  const redirect = stringParam(route.query.redirect)
  if (!redirect || typeof redirect !== 'string') {
    return ADMIN_DEFAULT_PATH
  }

  const safeRedirect = normalizeSafeInternalRedirectPath(redirect)
  if (!safeRedirect || !isPathUnderRoute(safeRedirect.pathname, ADMIN_ROOT_PATH)) {
    return ADMIN_DEFAULT_PATH
  }
  if (isPathUnderRoute(safeRedirect.pathname, ADMIN_LOGIN_PATH)) {
    return ADMIN_DEFAULT_PATH
  }

  return safeRedirect.fullPath
}

function goUserLogin() {
  void router.push(USER_LOGIN_PATH)
}

async function handleLogin() {
  formError.value = ''
  const email = formValue.value.email.trim()
  const password = formValue.value.password
  if (!email || !password) {
    formError.value = !email
      ? ADMIN_LOGIN_MESSAGES.emailRequired
      : ADMIN_LOGIN_MESSAGES.passwordRequired
    return
  }

  try {
    loading.value = true
    await userStore.login(email, password)
    if (userStore.isAdmin) {
      feedback.succeeded()
      await router.replace(resolveAdminRedirectPath())
    } else {
      const logoutSuccess = await logoutAndRedirect(ADMIN_LOGIN_PATH, { replace: true })
      if (!logoutSuccess) {
        feedback.sessionRevokeFailed()
      }
      feedback.nonAdminAccount()
    }
  } catch (error: unknown) {
    feedback.failed(error)
  } finally {
    loading.value = false
  }
}
</script>
