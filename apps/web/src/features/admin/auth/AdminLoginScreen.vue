<template>
  <div class="flex min-h-screen items-center justify-center bg-[#0a0a0b] p-6 max-[480px]:p-4">
    <div class="grid w-[min(900px,100%)] grid-cols-[1.02fr_1fr] gap-3 max-[900px]:grid-cols-1">
      <section
        class="rounded-[18px] border border-[#232326] bg-[#111214] p-[30px] max-[900px]:rounded-2xl max-[900px]:p-6"
      >
        <h1 class="m-0 text-[28px] text-[#f4f4f5] max-[480px]:text-2xl">管理员控制台入口</h1>
        <p class="mt-3.5 mb-0 max-w-[440px] text-sm leading-[1.72] text-[#a1a1aa]">
          用于账户、渠道、资源与任务管理。
        </p>
      </section>

      <section
        class="rounded-[18px] border border-[#232326] bg-[#101113] p-[30px] max-[900px]:rounded-2xl max-[900px]:p-6"
      >
        <h2 class="m-0 text-2xl text-[#f4f4f5]">管理员登录</h2>

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

        <n-button
          type="primary"
          block
          size="large"
          :loading="loading"
          :disabled="loading"
          @click="handleLogin"
        >
          {{ loading ? '登录中...' : '进入管理员后台' }}
        </n-button>

        <div class="mt-[18px] text-center text-[13px]">
          <router-link to="/user/login">用户登录</router-link>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const message = useMessage()

const formRef = ref()
const loading = ref(false)
const formValue = ref({
  username: '',
  password: '',
})

const rules = {
  username: { required: true, message: '请输入账号', trigger: 'blur' },
  password: { required: true, message: '请输入密码', trigger: 'blur' },
}

function resolveAdminRedirectPath() {
  const redirect = Array.isArray(route.query.redirect)
    ? route.query.redirect[0]
    : route.query.redirect
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
:deep(.n-form-item) {
  margin-bottom: 18px;
}

:deep(.n-form-item-label__text) {
  color: #d4d4d8;
}

:deep(.n-input) {
  --n-border: 1px solid #2b2c2f;
  --n-border-hover: 1px solid #3a3b40;
  --n-border-focus: 1px solid #3a3b40;
  --n-box-shadow-focus: none;
  --n-color: #151618;
  --n-color-focus: #151618;
  --n-text-color: #f4f4f5;
  --n-placeholder-color: #6b6b71;
  border-radius: 12px;
}

:deep(.n-input:focus),
:deep(.n-input:focus-visible),
:deep(.n-input.n-input--focus),
:deep(.n-input .n-input__state-border) {
  box-shadow: none !important;
  outline: none !important;
}

:deep(.n-button--primary-type) {
  --n-color: #f4f4f5;
  --n-color-hover: #ffffff;
  --n-color-pressed: #d4d4d8;
  --n-border: 1px solid #f4f4f5;
  --n-ripple-color: transparent;
  --n-text-color: #111111;
  border-radius: 12px;
  height: 48px;
  margin-top: 6px;
}

:deep(.n-button:focus),
:deep(.n-button:focus-visible),
:deep(.n-button .n-button__state-border) {
  box-shadow: none !important;
  outline: none !important;
}

a {
  color: #f4f4f5;
}
</style>
