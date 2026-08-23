<template>
  <div aria-live="polite">
    <AuthModalHost :busy="busy" @close="closeAuth">
      <LoginAuthPanel
        v-if="mode === 'login'"
        :oauth-redirect-path="oauthRedirectPath"
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
  HOME_PATH,
  SHARE_JOIN_PATH,
  USER_BOARDS_PATH,
  USER_CANVAS_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
} from '../../../shared/constants/routePaths'
import AuthModalHost from './AuthModalHost.vue'
import LoginAuthPanel from './LoginAuthPanel.vue'
import RegisterAuthPanel from './RegisterAuthPanel.vue'

const props = withDefaults(defineProps<{
  mode: 'login' | 'register'
  presentation?: 'overlay' | 'route'
}>(), {
  presentation: 'route',
})

const emit = defineEmits<{
  'overlay-close': []
  'overlay-success': []
  'update:mode': [mode: 'login' | 'register']
}>()

const router = useRouter()
const route = useRoute()
const busy = ref(false)

function goLogin() {
  busy.value = false
  if (props.presentation === 'overlay') {
    emit('update:mode', 'login')
    return
  }
  void router.replace({ path: USER_LOGIN_PATH, query: authRedirectQuery() })
}

function goRegister() {
  busy.value = false
  if (props.presentation === 'overlay') {
    emit('update:mode', 'register')
    return
  }
  void router.replace({ path: USER_REGISTER_PATH, query: authRedirectQuery() })
}

function authRedirectQuery(): Record<string, string> {
  const redirect = route.query.redirect
  const value = Array.isArray(redirect) ? redirect[0] : redirect
  return typeof value === 'string' && value ? { redirect: value } : {}
}

function resolveLoginRedirectPath() {
  const redirect = route.query.redirect
  const redirectPath = Array.isArray(redirect) ? redirect[0] : redirect
  const normalizedPath = typeof redirectPath === 'string' ? redirectPath.trim() : ''
  if (!normalizedPath) {
    return USER_BOARDS_PATH
  }
  if (!isSafeInternalRedirectPath(normalizedPath)) {
    return USER_BOARDS_PATH
  }
  return normalizedPath
}

function onLoginSuccess() {
  if (props.presentation === 'overlay') {
    emit('overlay-success')
    return
  }
  void router.replace(resolveLoginRedirectPath())
}

function onRegisterSuccess() {
  if (props.presentation === 'overlay') {
    emit('overlay-success')
    return
  }
  void router.replace(resolveLoginRedirectPath())
}

function closeAuth() {
  if (busy.value) return
  if (props.presentation === 'overlay') {
    emit('overlay-close')
    return
  }
  void router.replace(resolveAuthClosePath())
}

const oauthRedirectPath = props.presentation === 'overlay' ? route.fullPath : undefined

function resolveAuthClosePath() {
  const target = resolveLoginRedirectPath()
  const protectedPrefixes = [USER_BOARDS_PATH, USER_CANVAS_PATH, SHARE_JOIN_PATH]
  return protectedPrefixes.some(
    (prefix) => target === prefix || target.startsWith(`${prefix}/`),
  )
    ? HOME_PATH
    : target
}
</script>
