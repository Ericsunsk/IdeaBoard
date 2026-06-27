<template>
  <div aria-live="polite">
    <LoginModal
      :show="showLogin"
      @update:show="onLoginVisibleChange"
      @to-register="goRegister"
      @login-success="onLoginSuccess"
    />

    <RegisterModal
      :show="showRegister"
      @update:show="onRegisterVisibleChange"
      @to-login="goLogin"
      @register-success="onRegisterSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { isSafeInternalRedirectPath } from '../../../shared/utils/auth'
import {
  USER_HOME_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
} from '../../../shared/constants/routePaths'

const props = defineProps<{
  mode: 'login' | 'register'
}>()

const router = useRouter()
const route = useRoute()
const skipCloseRedirectOnce = ref(false)

const showLogin = computed(() => props.mode === 'login')
const showRegister = computed(() => props.mode === 'register')

function goLogin() {
  void router.replace(USER_LOGIN_PATH)
}

function goRegister() {
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
  skipCloseRedirectOnce.value = true
  void router.replace(resolveLoginRedirectPath())
}

function onRegisterSuccess() {
  skipCloseRedirectOnce.value = true
  void router.replace(resolveLoginRedirectPath())
}

function onLoginVisibleChange(visible: boolean) {
  if (visible) return
  if (skipCloseRedirectOnce.value) {
    skipCloseRedirectOnce.value = false
    return
  }
  void router.replace(USER_HOME_PATH)
}

function onRegisterVisibleChange(visible: boolean) {
  if (visible) return
  if (skipCloseRedirectOnce.value) {
    skipCloseRedirectOnce.value = false
    return
  }
  // 关闭注册弹窗时回到公开首页
  void router.replace(USER_HOME_PATH)
}
</script>
