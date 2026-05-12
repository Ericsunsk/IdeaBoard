<template>
  <div class="relative min-h-screen w-full overflow-hidden bg-[#f8f9fd]">
    <div class="auth-bg"></div>
    <div
      class="pointer-events-none absolute top-[42px] left-1/2 z-[1] -translate-x-1/2 text-center"
    >
      <h1 class="text-[30px] leading-[1.1] font-bold text-[#222a3c]">IdeaBoard</h1>
      <p class="mt-1.5 text-[13px] text-[#6f768a]">登录 IdeaBoard，继续你的创作流程。</p>
    </div>

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

<style scoped>
.auth-bg {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(40% 30% at 50% 20%, rgba(146, 120, 230, 0.14) 0%, rgba(146, 120, 230, 0) 100%),
    radial-gradient(circle, #e2e4e8 1.5px, transparent 1.5px);
  background-size:
    100% 100%,
    24px 24px;
  opacity: 1;
}
</style>
