<template>
  <div class="grid min-h-screen place-items-center bg-ib-page p-6 text-ib-text max-[520px]:p-4">
    <Card :class="loginCardClass" :pt="loginCardPt">
      <template #header>
        <router-link :class="brandMarkClass" to="/" :aria-label="projectName">
          <SiteBrandLogo
            :src="logoURL"
            :fallback-text="fallbackText"
            :class="brandMarkClasses.logo"
          />
        </router-link>
      </template>

      <template #title>
        <h1 class="m-0 text-admin-modal-title leading-[1.2] font-bold text-admin-title-color">
          管理员登录
        </h1>
      </template>

      <template #content>
        <form class="flex flex-col gap-4" novalidate @submit.prevent="handleLogin">
          <label class="flex flex-col gap-2 text-xs font-bold text-admin-text-2">
            <span>Email</span>
            <IconField>
              <InputIcon>
                <User class="h-4 w-4 text-admin-text-3" />
              </InputIcon>
              <InputText
                id="admin-login-email"
                v-model="formValue.email"
                name="email"
                type="email"
                autocomplete="email"
                class="!min-h-11"
                fluid
                placeholder="admin@example.com"
              />
            </IconField>
          </label>

          <label class="flex flex-col gap-2 text-xs font-bold text-admin-text-2">
            <span>密码</span>
            <Password
              v-model="formValue.password"
              autocomplete="current-password"
              :input-props="{
                id: 'admin-login-password',
                name: 'password',
                autocomplete: 'current-password',
              }"
              toggle-mask
              :feedback="false"
              class="w-full"
              input-class="!min-h-11"
              fluid
              placeholder="password"
            />
          </label>

          <p v-if="formError" class="m-0 text-admin-body font-semibold text-admin-danger">
            {{ formError }}
          </p>

          <Button
            type="submit"
            :class="adminActionButtonClass"
            severity="contrast"
            :label="loading ? '登录中' : '进入后台'"
            :loading="loading"
            fluid
          />
        </form>
      </template>

      <template #footer>
        <Button
          label="用户登录"
          text
          severity="secondary"
          :class="adminTextButtonClass"
          @click="goUserLogin"
        />
      </template>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../auth/session/userSession'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ADMIN_LOGIN_PATH,
  ADMIN_ROOT_PATH,
  USER_LOGIN_PATH,
  isPathUnderRoute,
} from '../../../../shared/constants/routePaths'
import { ADMIN_DEFAULT_PATH } from '../../app/navigationMeta'
import { User } from '@iconoir/vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import SiteBrandLogo from '../../../../shared/components/SiteBrandLogo.vue'
import { useLogoutRedirect } from '../../../../shared/composables/useLogoutRedirect'
import { useSiteBranding } from '../../../../shared/composables/useSiteBranding'
import { normalizeSafeInternalRedirectPath } from '../../../../shared/utils/auth'
import { brandMarkClasses, createBrandMarkClass } from '../../../../shared/lib/brandMark'
import { adminActionButtonClass, adminTextButtonClass } from '../../shared/lib/uiClasses'
import { useAdminLoginFeedback } from './useAdminLoginFeedback'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const feedback = useAdminLoginFeedback()
const { fallbackText, logoURL, projectName } = useSiteBranding()
const brandMarkClass = computed(() =>
  createBrandMarkClass(
    brandMarkClasses.sizes.login,
    logoURL.value ? brandMarkClasses.states.image : brandMarkClasses.states.fallbackStrong,
  ),
)
const loginCardClass =
  'admin-login-card w-[min(420px,100%)] overflow-hidden rounded-[18px] border border-ib-border bg-ib-surface shadow-[0_24px_70px_rgba(45,64,89,0.12)]'

const loading = ref(false)
const formError = ref('')
const formValue = ref({
  email: '',
  password: '',
})
const loginCardPt = {
  header: {
    class: 'flex justify-center !px-8 !pt-8 !pb-2 max-[520px]:!px-6 max-[520px]:!pt-[26px]',
  },
  body: {
    class: '!px-8 !pt-0 !pb-7 max-[520px]:!px-6 max-[520px]:!pb-6',
  },
  caption: { class: '!p-0 text-center' },
  content: { class: '!pt-6 !pb-0' },
  footer: { class: 'flex justify-center !pt-[18px] !pb-0' },
} as const

function resolveAdminRedirectPath() {
  const redirect = Array.isArray(route.query.redirect)
    ? route.query.redirect[0]
    : route.query.redirect
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
    formError.value = !email ? '请输入 Email' : '请输入密码'
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
