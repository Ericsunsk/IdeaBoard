<template>
  <div class="auth-page">
    <div class="auth-bg"></div>
    <div class="auth-brand">
      <h1>IdeaBoard</h1>
      <p>Sign in to continue</p>
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
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import LoginModal from '../components/LoginModal.vue'
import RegisterModal from '../components/RegisterModal.vue'

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
.auth-page {
  width: 100%;
  min-height: 100vh;
  position: relative;
  overflow: hidden;
  background: #f7f8fc;
}

.auth-bg {
  position: absolute;
  inset: 0;
  background-image: radial-gradient(circle, #e2e4e8 1.5px, transparent 1.5px);
  background-size: 24px 24px;
  opacity: 0.9;
}

.auth-brand {
  position: absolute;
  top: 42px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 1;
  text-align: center;
  pointer-events: none;
}

.auth-brand h1 {
  font-size: 30px;
  line-height: 1.1;
  font-weight: 700;
  color: #2a3142;
}

.auth-brand p {
  margin-top: 6px;
  font-size: 13px;
  color: #7a8295;
}
</style>
