<template>
  <div class="login-page">
    <div class="login-card">
      <div class="logo">🛡️</div>
      <h1>管理员登录</h1>
      <p class="subtitle">请输入管理员账号信息</p>

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

      <n-button type="error" block size="large" :loading="loading" :disabled="loading" @click="handleLogin">
        {{ loading ? '登录中...' : '管理员登录' }}
      </n-button>

      <div class="footer">
        <span>普通用户？</span>
        <router-link to="/user/login">用户登录</router-link>
      </div>
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
  display: flex;
  align-items: center;
  justify-content: center;
  background: #09090b;
}

.login-card {
  width: 100%;
  max-width: 380px;
  padding: 40px;
  background: #18181b;
  border-radius: 12px;
  border: 1px solid #27272a;
  text-align: center;
}

.logo {
  font-size: 48px;
  margin-bottom: 16px;
}

.login-card h1 {
  font-size: 24px;
  font-weight: 600;
  color: #fafafa;
  margin-bottom: 8px;
}

.subtitle {
  color: #71717a;
  font-size: 14px;
  margin-bottom: 32px;
}

:deep(.n-form-item) {
  margin-bottom: 20px;
  text-align: left;
}

:deep(.n-form-item-label__text) {
  font-size: 14px;
  color: #a1a1aa;
}

:deep(.n-input) {
  --n-border: 1px solid #3f3f46;
  --n-border-hover: 1px solid #ef4444;
  --n-border-focus: 1px solid #ef4444;
  --n-color: #27272a;
  --n-color-focus: #27272a;
  --n-text-color: #fafafa;
  --n-placeholder-color: #71717a;
  --n-caret-color: #ef4444;
  border-radius: 8px;
}

:deep(.n-button--error-type) {
  --n-color: #ef4444;
  --n-color-hover: #f87171;
  --n-color-pressed: #dc2626;
  --n-color-focus: #ef4444;
  height: 44px;
  font-size: 15px;
  border-radius: 8px;
  margin-top: 8px;
}

.footer {
  margin-top: 24px;
  text-align: center;
  color: #71717a;
  font-size: 14px;
}

.footer a {
  color: #ef4444;
  margin-left: 8px;
}
</style>
