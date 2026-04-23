<template>
  <div class="login-page">
    <div class="login-shell">
      <section class="login-intro">
        <h1>管理员控制台入口</h1>
        <p>用于账户、渠道、资源与任务管理。</p>
      </section>

      <section class="login-card">
        <h2>管理员登录</h2>

        <n-form ref="formRef" :model="formValue" :rules="rules" size="large" label-placement="top">
          <n-form-item path="username" label="账号">
            <n-input v-model:value="formValue.username" placeholder="请输入管理员账号" />
          </n-form-item>

          <n-form-item path="password" label="密码">
            <n-input
              v-model:value="formValue.password"
              type="password"
              placeholder="请输入密码"
              show-password-on="click"
              @keyup.enter="handleLogin"
            />
          </n-form-item>
        </n-form>

        <n-button type="primary" block size="large" :loading="loading" :disabled="loading" @click="handleLogin">
          {{ loading ? '登录中...' : '进入管理员后台' }}
        </n-button>

        <div class="footer">
          <router-link to="/user/login">用户登录</router-link>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../../../app/stores/user'
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
      const logoutSuccess = await userStore.logout('/admin/login')
      if (!logoutSuccess) {
        message.warning('会话撤销失败，请稍后重试')
      }
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
  padding: 24px;
  background: #0a0a0b;
}

.login-shell {
  width: min(900px, 100%);
  display: grid;
  grid-template-columns: 1.02fr 1fr;
  gap: 12px;
}

.login-intro,
.login-card {
  border: 1px solid #232326;
  border-radius: 18px;
  padding: 30px;
}

.login-intro {
  background: #111214;
}

.login-intro h1 {
  margin: 0;
  font-size: 28px;
  color: #f4f4f5;
}

.login-intro p {
  margin: 14px 0 0;
  color: #a1a1aa;
  line-height: 1.72;
  font-size: 14px;
  max-width: 440px;
}

.login-card {
  background: #101113;
}

.login-card h2 {
  margin: 0;
  font-size: 24px;
  color: #f4f4f5;
}

:deep(.n-form-item) {
  margin-bottom: 18px;
}

:deep(.n-form-item-label__text) {
  color: #d4d4d8;
}

:deep(.n-input) {
  --n-border: 1px solid #2b2c2f;
  --n-border-hover: 1px solid #3a3b40;
  --n-border-focus: 1px solid #e4e4e7;
  --n-color: #151618;
  --n-color-focus: #151618;
  --n-text-color: #f4f4f5;
  --n-placeholder-color: #6b6b71;
  border-radius: 12px;
}

:deep(.n-button--primary-type) {
  --n-color: #f4f4f5;
  --n-color-hover: #ffffff;
  --n-color-pressed: #d4d4d8;
  --n-border: 1px solid #f4f4f5;
  --n-text-color: #111111;
  border-radius: 12px;
  height: 48px;
  margin-top: 6px;
}

.footer {
  margin-top: 18px;
  text-align: center;
  font-size: 13px;
}

.footer a {
  color: #f4f4f5;
}

@media (max-width: 900px) {
  .login-shell {
    grid-template-columns: 1fr;
  }

  .login-intro,
  .login-card {
    padding: 24px;
    border-radius: 16px;
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
