<template>
  <div aria-live="polite">
    <AuthModalHost :busy="busy" @close="closeAuth">
      <LoginAuthPanel
        v-if="mode === 'login'"
        @close="closeAuth"
        @to-register="goRegister"
        @login-success="onLoginSuccess"
        @update:busy="busy = $event"
      />
      <RegisterAuthPanel
        v-else
        @close="closeAuth"
        @to-login="goLogin"
        @register-success="onRegisterSuccess"
        @update:busy="busy = $event"
      />
    </AuthModalHost>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { isSafeInternalRedirectPath } from '../../../shared/utils/auth'
import {
  USER_HOME_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
} from '../../../shared/constants/routePaths'
import AuthModalHost from './AuthModalHost.vue'
import LoginAuthPanel from './LoginAuthPanel.vue'
import RegisterAuthPanel from './RegisterAuthPanel.vue'

defineProps<{
  mode: 'login' | 'register'
}>()

const router = useRouter()
const route = useRoute()
const busy = ref(false)

function goLogin() {
  busy.value = false
  void router.replace(USER_LOGIN_PATH)
}

function goRegister() {
  busy.value = false
  void router.replace(USER_REGISTER_PATH)
}

function resolveLoginRedirectPath() {
  const redirect = route.query.redirect
  const redirectPath = Array.isArray(redirect) ? redirect[0] : redirect
  const normalizedPath = typeof redirectPath === 'string' ? redirectPath.trim() : ''
  if (!normalizedPath) {
    return USER_HOME_PATH
  }
  if (!isSafeInternalRedirectPath(normalizedPath)) {
    return USER_HOME_PATH
  }
  return normalizedPath
}

function onLoginSuccess() {
  void router.replace(resolveLoginRedirectPath())
}

function onRegisterSuccess() {
  void router.replace(resolveLoginRedirectPath())
}

function closeAuth() {
  if (busy.value) return
  void router.replace(USER_HOME_PATH)
}
</script>
