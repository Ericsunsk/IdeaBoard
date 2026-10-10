<template>
  <div aria-live="polite">
    <AuthModalHost
      :busy="busy"
      :aria-label="mode === 'login' ? t('auth.signIn') : t('auth.createAccount')"
      :description="mode === 'login' ? t('auth.signIn') : t('auth.createAccount')"
      @close="closeAuth"
    >
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
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { isSafeInternalRedirectPath } from '../../../core/utils/auth'
import { stringParam, stringParamWhere } from '../../../core/router/queryParams'
import {
  HOME_PATH,
  SHARE_JOIN_PATH,
  USER_BOARDS_PATH,
  USER_CANVAS_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
} from '../../workspace/routePaths'
import AuthModalHost from './AuthModalHost.vue'
import LoginAuthPanel from './LoginAuthPanel.vue'
import RegisterAuthPanel from './RegisterAuthPanel.vue'

const props = withDefaults(
  defineProps<{
    mode: 'login' | 'register'
    presentation?: 'overlay' | 'route'
  }>(),
  {
    presentation: 'route',
  },
)

const emit = defineEmits<{
  'overlay-close': []
  'overlay-success': []
  'update:mode': [mode: 'login' | 'register']
}>()

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const busy = ref(false) // Overlay dismiss guard (not `useAsyncAction`): read by the modal lifecycle.

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
  const value = stringParamWhere(route.query.redirect, (candidate) => candidate !== '')
  return value === undefined ? {} : { redirect: value }
}

function resolveLoginRedirectPath() {
  const redirectPath = stringParam(route.query.redirect)
  const normalizedPath = redirectPath?.trim() ?? ''
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
  return protectedPrefixes.some((prefix) => target === prefix || target.startsWith(`${prefix}/`))
    ? HOME_PATH
    : target
}
</script>
