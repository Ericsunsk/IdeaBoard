<template>
  <div class="register-page">
    <div class="register-card">
      <h1>注册</h1>
      <p class="subtitle">创建一个新账号</p>

      <n-form ref="formRef" :model="formValue" :rules="rules" size="large">
        <n-form-item path="username" label="账号">
          <n-input v-model:value="formValue.username" placeholder="请输入账号（3-20个字符）" />
        </n-form-item>

        <n-form-item path="password" label="密码">
          <n-input
            v-model:value="formValue.password"
            type="password"
            placeholder="请输入密码（至少6个字符）"
            show-password-on="click"
          />
        </n-form-item>

        <n-form-item path="confirmPassword" label="确认密码">
          <n-input
            v-model:value="formValue.confirmPassword"
            type="password"
            placeholder="请再次输入密码"
            show-password-on="click"
          />
        </n-form-item>
      </n-form>

      <n-button type="primary" block size="large" :loading="loading" :disabled="loading" @click="handleRegister">
        {{ loading ? '注册中...' : '注册' }}
      </n-button>

      <div class="footer">
        <span>已有账号？</span>
        <router-link to="/user/login">立即登录</router-link>
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
  password: '',
  confirmPassword: ''
})

const validateConfirmPassword = (rule: any, value: string) => {
  if (value !== formValue.value.password) {
    return new Error('两次输入的密码不一致')
  }
  return true
}

const rules = {
  username: [
    { required: true, message: '请输入账号', trigger: 'blur' },
    { min: 3, max: 20, message: '账号长度需在3-20个字符之间', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '密码至少6个字符', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入密码', trigger: 'blur' },
    { validator: validateConfirmPassword, trigger: 'blur' }
  ]
}

async function handleRegister() {
  try {
    await formRef.value?.validate()
    loading.value = true
    await userStore.register(formValue.value.username, formValue.value.password)
    message.success('注册成功，请登录')
    router.push('/user/login')
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
.register-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #09090b;
}

.register-card {
  width: 100%;
  max-width: 380px;
  padding: 40px;
  background: #18181b;
  border-radius: 12px;
  border: 1px solid #27272a;
}

.register-card h1 {
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
