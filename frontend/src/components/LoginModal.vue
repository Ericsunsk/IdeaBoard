<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="show" class="login-modal-overlay" @click.self="handleClose">
        <div class="login-modal-content">
          <button class="close-btn" :disabled="loading" @click="handleClose">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>
          
          <div class="modal-header">
            <h2 class="gradient-text">Welcome Back</h2>
            <p>Please sign in to your account</p>
          </div>

          <n-form ref="formRef" :model="formValue" :rules="rules" size="large">
            <n-form-item path="username" label="Username">
              <n-input v-model:value="formValue.username" class="apple-input" />
            </n-form-item>

            <n-form-item path="password" label="Password">
              <n-input
                v-model:value="formValue.password"
                type="password"
                show-password-on="click"
                class="apple-input"
              />
            </n-form-item>
          </n-form>

          <n-button type="primary" block size="large" :loading="loading" :disabled="loading" class="apple-login-btn" @click="handleLogin">
            {{ loading ? 'Signing in...' : 'Sign in' }}
          </n-button>

          <div class="modal-footer">
            <span>Don't have an account?</span>
            <button type="button" class="signup-link" :disabled="loading" @click="handleSwitchToRegister">Sign up</button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useUserStore } from '../stores/user'
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'
import type { FormInst } from 'naive-ui'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'to-register'): void
  (e: 'login-success'): void
}>()

const userStore = useUserStore()
const message = useMessage()

const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const formValue = ref({
  username: '',
  password: ''
})

const ASCII_VISIBLE_NO_SPACE_REGEX = /^[\x21-\x7E]+$/

function sanitizeCredentialInput(value: string) {
  return value.replace(/[^\x21-\x7E]/g, '')
}

function validateCredentialChars(_: unknown, value: string) {
  if (!value) return true
  return ASCII_VISIBLE_NO_SPACE_REGEX.test(value)
}

const rules = {
  username: [
    { required: true, message: 'Please enter your username', trigger: 'blur' },
    { validator: validateCredentialChars, message: 'Only letters, numbers, and symbols are allowed', trigger: ['input', 'blur'] }
  ],
  password: [
    { required: true, message: 'Please enter your password', trigger: 'blur' },
    { validator: validateCredentialChars, message: 'Only letters, numbers, and symbols are allowed', trigger: ['input', 'blur'] }
  ]
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
  }
)

watch(
  () => formValue.value.username,
  (value) => {
    const sanitized = sanitizeCredentialInput(value)
    if (sanitized !== value) {
      formValue.value.username = sanitized
    }
  }
)

watch(
  () => formValue.value.password,
  (value) => {
    const sanitized = sanitizeCredentialInput(value)
    if (sanitized !== value) {
      formValue.value.password = sanitized
    }
  }
)

async function handleLogin() {
  try {
    await formRef.value?.validate()
    loading.value = true
    await userStore.login(formValue.value.username, formValue.value.password)
    message.success('Sign in successful')
    resetForm()
    emit('login-success')
    emit('update:show', false)
  } catch (error: any) {
    if (Array.isArray(error)) return
    message.error(error?.message || 'Sign in failed, please try again.')
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
  background: rgba(0, 0, 0, 0.4);
  backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.login-modal-content {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(20px);
  width: 100%;
  max-width: 440px;
  padding: 48px;
  border-radius: 32px;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);
  position: relative;
  border: 1px solid rgba(255, 255, 255, 0.3);
  animation: modalIn 0.5s cubic-bezier(0.16, 1, 0.3, 1);
}

@keyframes modalIn {
  from { opacity: 0; transform: scale(0.95) translateY(20px); }
  to { opacity: 1; transform: scale(1) translateY(0); }
}

.close-btn {
  position: absolute;
  top: 24px;
  right: 24px;
  background: rgba(0, 0, 0, 0.05);
  border: none;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #1d1d1f;
  cursor: pointer;
  transition: all 0.2s;
}

.close-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.close-btn:hover {
  background: rgba(0, 0, 0, 0.1);
  transform: scale(1.1);
}

.modal-header {
  text-align: center;
  margin-bottom: 32px;
}

.gradient-text {
  font-size: 32px;
  font-weight: 700;
  margin-bottom: 8px;
  background: linear-gradient(135deg, #1d1d1f 0%, #434343 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  letter-spacing: -0.02em;
}

.modal-header p {
  color: #86868b;
  font-size: 16px;
}

.apple-input :deep(.n-input-wrapper) {
  padding-left: 12px;
  padding-right: 12px;
}

.apple-input :deep(.n-input) {
  --n-border: 1px solid #d2d2d7;
  --n-border-hover: 1px solid #9278e6;
  --n-border-focus: 1px solid #9278e6;
  --n-color: rgba(255, 255, 255, 0.8);
  --n-color-focus: #ffffff;
  --n-text-color: #1d1d1f !important;
  --n-placeholder-color: #86868b;
  --n-caret-color: #9278e6;
  border-radius: 14px;
  font-size: 16px;
  height: 52px;
}

.apple-input :deep(input) {
  color: #1d1d1f !important;
}

:deep(.n-form-item-label__text) {
  font-size: 14px;
  font-weight: 500;
  color: #1d1d1f;
  margin-left: 4px;
}

.apple-login-btn {
  background: #9278e6 !important;
  color: #ffffff !important;
  height: 52px !important;
  border-radius: 16px !important;
  font-size: 17px !important;
  font-weight: 600 !important;
  margin-top: 12px !important;
  border: none !important;
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1) !important;
  box-shadow: 0 8px 24px rgba(146, 120, 230, 0.3) !important;
}

.apple-login-btn:hover {
  background: #8065d6 !important;
  transform: translateY(-2px);
  box-shadow: 0 12px 32px rgba(146, 120, 230, 0.4) !important;
}

.apple-login-btn:active {
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
  color: #9278e6;
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
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from, .fade-leave-to {
  opacity: 0;
}
</style>
