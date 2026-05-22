<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="show" class="login-modal-overlay" @click.self="handleClose">
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
          class="login-modal-content"
        >
          <AuthModalCloseButton :disabled="loading" @click="handleClose" />

          <div class="modal-header">
            <h2 class="heading-text">欢迎回来</h2>
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

          <div class="modal-footer">
            <span>还没有账号？</span>
            <button
              type="button"
              class="signup-link"
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

<style scoped>
.login-modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background: rgba(21, 27, 43, 0.24);
  backdrop-filter: blur(6px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.login-modal-content {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(14px);
  width: 100%;
  max-width: 440px;
  padding: 48px;
  border-radius: 22px;
  box-shadow: 0 24px 40px rgba(34, 45, 76, 0.16);
  position: relative;
  border: 1px solid #d9dfec;
}

.modal-header {
  text-align: center;
  margin-bottom: 32px;
}

.heading-text {
  font-size: 32px;
  font-weight: 700;
  margin-bottom: 8px;
  background: linear-gradient(135deg, #2b3142 0%, #4f5a76 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  letter-spacing: -0.02em;
}

.modal-header p {
  color: #86868b;
  font-size: 16px;
}

.auth-input :deep(.n-input-wrapper) {
  padding-left: 12px;
  padding-right: 12px;
}

.auth-input :deep(.n-input) {
  --n-border: 1px solid #d2d2d7;
  --n-border-hover: 1px solid #7a6bd5;
  --n-border-focus: 1px solid #7a6bd5;
  --n-color: rgba(255, 255, 255, 0.8);
  --n-color-focus: #ffffff;
  --n-text-color: #1d1d1f !important;
  --n-placeholder-color: #86868b;
  --n-caret-color: #7a6bd5;
  border-radius: 14px;
  font-size: 16px;
  height: 52px;
}

.auth-input :deep(input) {
  color: #1d1d1f !important;
}

:deep(.n-form-item-label__text) {
  font-size: 14px;
  font-weight: 500;
  color: #1d1d1f;
  margin-left: 4px;
}

.auth-primary-btn {
  background: #7a6bd5 !important;
  color: #ffffff !important;
  height: 52px !important;
  border-radius: 16px !important;
  font-size: 17px !important;
  font-weight: 600 !important;
  margin-top: 12px !important;
  border: none !important;
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1) !important;
  box-shadow: 0 8px 24px rgba(122, 107, 213, 0.28) !important;
}

.auth-primary-btn:hover {
  background: #6d5fd0 !important;
  transform: translateY(-2px);
  box-shadow: 0 12px 32px rgba(122, 107, 213, 0.36) !important;
}

.auth-primary-btn:active {
  transform: translateY(0);
}

.modal-footer {
  margin-top: 32px;
  text-align: center;
  font-size: 15px;
  color: #86868b;
}

.signup-link {
  background: none;
  border: none;
  color: #7a6bd5;
  font-weight: 600;
  margin-left: 8px;
  text-decoration: none;
  cursor: pointer;
  font-size: inherit;
  padding: 0;
}

.signup-link:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.signup-link:hover {
  text-decoration: underline;
}

/* Transitions */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
