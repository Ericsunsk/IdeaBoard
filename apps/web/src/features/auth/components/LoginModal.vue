<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="show" class="auth-modal-overlay" @click.self="handleClose">
        <div
          v-motion="{
            initial: { opacity: 0, y: 20, scale: 0.96 },
            enter: {
              opacity: 1,
              y: 0,
              scale: 1,
              transition: { duration: 260, easing: 'ease-out' },
            },
          }"
          class="auth-modal-content login-modal-content"
        >
          <AuthModalCloseButton :disabled="loading" @click="handleClose" />

          <div class="auth-modal-header login-modal-header">
            <h2 class="auth-modal-title">欢迎回来</h2>
            <p>登录 IdeaBoard，继续你的创作</p>
          </div>

          <n-form ref="formRef" :model="formValue" :rules="rules" size="large">
            <n-form-item path="username" label="账号">
              <n-input
                v-model:value="formValue.username"
                placeholder="请输入账号"
                class="auth-input"
              />
            </n-form-item>

            <n-form-item path="password" label="密码">
              <n-input
                v-model:value="formValue.password"
                type="password"
                show-password-on="click"
                placeholder="请输入密码"
                class="auth-input"
              />
            </n-form-item>
          </n-form>

          <n-button
            type="primary"
            block
            size="large"
            :loading="loading"
            :disabled="loading"
            class="auth-primary-btn"
            @click="handleLogin"
          >
            {{ loading ? '登录中...' : '登录' }}
          </n-button>

          <div class="auth-modal-footer login-modal-footer">
            <span>还没有账号？</span>
            <button
              type="button"
              class="auth-modal-switch-link"
              :disabled="loading"
              @click="handleSwitchToRegister"
            >
              立即注册
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { useAppToast } from '../../../shared/composables/useAppToast'
import type { FormInst } from 'naive-ui'
import AuthModalCloseButton from './AuthModalCloseButton.vue'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'to-register'): void
  (e: 'login-success'): void
}>()

const userStore = useUserStore()
const appToast = useAppToast()

const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const formValue = ref({
  username: '',
  password: '',
})

const rules = {
  username: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

function resetForm() {
  formValue.value.username = ''
  formValue.value.password = ''
  formRef.value?.restoreValidation()
}

function handleClose() {
  if (loading.value) return
  emit('update:show', false)
  resetForm()
}

function handleSwitchToRegister() {
  if (loading.value) return
  emit('to-register')
  resetForm()
}

watch(
  () => props.show,
  (visible) => {
    if (!visible) {
      resetForm()
    }
  },
)

async function handleLogin() {
  try {
    await formRef.value?.validate()
    loading.value = true
    await userStore.login(formValue.value.username, formValue.value.password)
    appToast.success('登录成功')
    resetForm()
    emit('login-success')
    emit('update:show', false)
  } catch (error: any) {
    if (Array.isArray(error)) return
    appToast.error(error?.message || '登录失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped src="./auth-modal.css"></style>

<style scoped>
.login-modal-content {
  padding: 48px;
}

.login-modal-header {
  margin-bottom: 32px;
}

.auth-input :deep(.n-input-wrapper) {
  padding-left: 12px;
  padding-right: 12px;
}

.login-modal-footer {
  margin-top: 32px;
}
</style>
