<template>
  <div class="login-page">
    <div class="login-card">
      <h1>登录</h1>
      <p class="subtitle">请输入您的账号信息</p>

      <n-form ref="formRef" :model="formValue" :rules="rules" size="large">
        <n-form-item path="username" label="账号">
          <n-input v-model:value="formValue.username" placeholder="请输入账号" />
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
        {{ loading ? '登录中...' : '登录' }}
      </n-button>

      <div class="footer">
        <span>还没有账号？</span>
        <router-link to="/user/register">立即注册</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '../../stores/user'
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'

const router = useRouter()
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

async function handleLogin() {
  try {
    await formRef.value?.validate()
    loading.value = true
    await userStore.login(formValue.value.username, formValue.value.password)
    message.success('登录成功')
    router.push('/user/home')
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
}

:deep(.n-form-item-label__text) {
  font-size: 14px;
  color: #a1a1aa;
}

:deep(.n-input) {
  --n-border: 1px solid #3f3f46;
  --n-border-hover: 1px solid #6366f1;
  --n-border-focus: 1px solid #6366f1;
  --n-color: #27272a;
  --n-color-focus: #27272a;
  --n-text-color: #fafafa;
  --n-placeholder-color: #71717a;
  --n-caret-color: #6366f1;
  border-radius: 8px;
}

:deep(.n-button--primary-type) {
  --n-color: #6366f1;
  --n-color-hover: #818cf8;
  --n-color-pressed: #4f46e5;
  --n-color-focus: #6366f1;
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
  color: #6366f1;
  margin-left: 8px;
}
</style>
