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
    />
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  mode: 'login' | 'register'
}>()

const router = useRouter()
const route = useRoute()
const skipCloseRedirectOnce = ref(false)

const showLogin = computed(() => props.mode === 'login')
const showRegister = computed(() => props.mode === 'register')

function goLogin() {
  void router.replace('/user/login')
}

function goRegister() {
  void router.replace('/user/register')
}

function resolveLoginRedirectPath() {
  const redirect = route.query.redirect
  const redirectPath = Array.isArray(redirect) ? redirect[0] : redirect
  if (!redirectPath || typeof redirectPath !== 'string') {
    return '/user/home'
  }
  if (!redirectPath.startsWith('/')) {
    return '/user/home'
  }
  if (redirectPath.startsWith('/user/login') || redirectPath.startsWith('/user/register')) {
    return '/user/home'
  }
  return redirectPath
}

function onLoginSuccess() {
  skipCloseRedirectOnce.value = true
  void router.replace(resolveLoginRedirectPath())
}

function onLoginVisibleChange(visible: boolean) {
  if (visible) return
  if (skipCloseRedirectOnce.value) {
    skipCloseRedirectOnce.value = false
    return
  }
  void router.replace('/user/home')
}

function onRegisterVisibleChange(visible: boolean) {
  if (visible) return
  // 关闭注册弹窗时回到公开首页
  void router.replace('/user/home')
}
</script>
