<template>
  <div class="login-page">
    <div class="mesh-bg">
      <div class="mesh-blob blob-1"></div>
      <div class="mesh-blob blob-2"></div>
      <div class="mesh-blob blob-3"></div>
    </div>

    <div class="login-shell">
      <section class="login-intro">
        <div class="intro-badge">FENG · Admin Only</div>
        <h1>管理员控制台入口</h1>
        <p>用于账户、渠道、资源与任务配置管理。请使用管理员账号登录，所有操作会即时生效。</p>
      </section>

      <section class="login-card">
        <h2>管理员登录</h2>
        <p class="subtitle">输入账号信息以进入控制台</p>

        <n-form ref="formRef" :model="formValue" :rules="rules" size="large">
          <n-form-item path="username" label="账号">
            <n-input v-model:value="formValue.username" placeholder="请输入管理员账号" />
          </n-form-item>

          <n-form-item path="password" label="密码">
            <n-input
              v-model:value="formValue.password"
              type="password"
              placeholder="请输入密码"
              show-password-on="click"
            />
          </n-form-item>
        </n-form>

        <n-button type="primary" block size="large" :loading="loading" :disabled="loading" @click="handleLogin">
          {{ loading ? '登录中...' : '进入管理员后台' }}
        </n-button>

        <div class="footer">
          <span>普通用户？</span>
          <router-link to="/user/login">用户登录</router-link>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../../stores/user'
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const message = useMessage()

const formRef = ref()
const loading = ref(false)
const formValue = ref({
  username: '',
  password: ''
})

const rules = {
  username: { required: true, message: '请输入账号', trigger: 'blur' },
  password: { required: true, message: '请输入密码', trigger: 'blur' }
}

function resolveAdminRedirectPath() {
  const redirect = Array.isArray(route.query.redirect) ? route.query.redirect[0] : route.query.redirect
  if (!redirect || typeof redirect !== 'string') {
    return '/admin/users'
  }
  if (!redirect.startsWith('/admin/') || redirect.startsWith('/admin/login')) {
    return '/admin/users'
  }
  return redirect
}

async function handleLogin() {
  try {
    await formRef.value?.validate()
    loading.value = true
    await userStore.login(formValue.value.username, formValue.value.password)
    if (userStore.isAdmin) {
      message.success('管理员登录成功')
      await router.replace(resolveAdminRedirectPath())
    } else {
      userStore.clearAuth()
      message.error('该账号不是管理员')
    }
  } catch (error: any) {
    if (error.message) {
      message.error(error.message)
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 28px;
  background: #f4f7ff;
}

.mesh-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.mesh-blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(46px);
  opacity: 0.5;
}

.blob-1 {
  width: 440px;
  height: 440px;
  background: #7d6fff;
  left: -140px;
  top: -140px;
}

.blob-2 {
  width: 400px;
  height: 400px;
  background: #5e9dff;
  right: -120px;
  top: 80px;
}

.blob-3 {
  width: 320px;
  height: 320px;
  background: #b7d4ff;
  left: 45%;
  bottom: -130px;
}

.login-shell {
  position: relative;
  z-index: 1;
  width: min(940px, 100%);
  display: grid;
  grid-template-columns: 1.08fr 1fr;
  gap: 18px;
}

.login-intro,
.login-card {
  border: 1px solid rgba(141, 155, 212, 0.3);
  border-radius: 32px;
  padding: 30px;
  backdrop-filter: blur(18px);
  box-shadow: 0 26px 64px rgba(73, 88, 150, 0.15);
}

.login-intro {
  background:
    radial-gradient(circle at right top, rgba(113, 97, 239, 0.2), transparent 45%),
    rgba(255, 255, 255, 0.7);
}

.intro-badge {
  display: inline-flex;
  align-items: center;
  padding: 6px 12px;
  border-radius: 999px;
  border: 1px solid rgba(113, 97, 239, 0.24);
  background: rgba(113, 97, 239, 0.1);
  color: #5f53d8;
  font-size: 12px;
  margin-bottom: 16px;
}

.login-intro h1 {
  margin: 0;
  font-size: 30px;
  color: #253057;
}

.login-intro p {
  margin: 14px 0 0;
  color: #5f698d;
  line-height: 1.72;
  font-size: 14px;
  max-width: 440px;
}

.login-card {
  background: rgba(255, 255, 255, 0.82);
}

.login-card h2 {
  margin: 0;
  font-size: 24px;
  color: #253057;
}

.subtitle {
  color: #7985ae;
  font-size: 13px;
  margin: 8px 0 20px;
}

:deep(.n-form-item) {
  margin-bottom: 18px;
}

:deep(.n-form-item-label__text) {
  color: #68749d;
}

:deep(.n-input) {
  --n-border: 1px solid rgba(136, 151, 206, 0.34);
  --n-border-hover: 1px solid rgba(113, 97, 239, 0.52);
  --n-border-focus: 1px solid rgba(113, 97, 239, 0.62);
  --n-color: rgba(255, 255, 255, 0.9);
  --n-color-focus: #ffffff;
  --n-text-color: #28335c;
  --n-placeholder-color: #98a3cb;
  border-radius: 14px;
}

:deep(.n-button--primary-type) {
  --n-color: linear-gradient(135deg, #7161ef, #4f8eff);
  --n-color-hover: linear-gradient(135deg, #6656ea, #4587f7);
  --n-color-pressed: linear-gradient(135deg, #5d4de0, #3d7eea);
  --n-border: 1px solid rgba(113, 97, 239, 0.9);
  border-radius: 16px;
  height: 48px;
  margin-top: 6px;
}

.footer {
  margin-top: 18px;
  text-align: center;
  color: #7d88b2;
  font-size: 13px;
}

.footer a {
  color: #5a60d3;
  margin-left: 6px;
}

@media (max-width: 900px) {
  .login-shell {
    grid-template-columns: 1fr;
  }

  .login-intro,
  .login-card {
    padding: 24px;
    border-radius: 24px;
  }
}

@media (max-width: 480px) {
  .login-page {
    padding: 16px;
  }

  .login-intro h1 {
    font-size: 24px;
  }
}
</style>
