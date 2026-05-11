<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="show" class="register-modal-overlay" @click.self="handleClose">
        <div class="register-modal-content">
          <button class="close-btn" :disabled="loading" @click="handleClose">
            <svg
              width="24"
              height="24"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>

          <div class="modal-header">
            <h2 class="heading-text">创建账号</h2>
            <p>创建你的 IdeaBoard 账号，开始创作</p>
          </div>

          <n-form ref="formRef" :model="formValue" :rules="rules" size="large">
            <n-form-item path="username" label="账号">
              <n-input
                v-model:value="formValue.username"
                placeholder="设置你的账号"
                class="auth-input"
              />
            </n-form-item>

            <n-form-item path="password" label="密码">
              <n-input
                v-model:value="formValue.password"
                type="password"
                placeholder="设置登录密码"
                show-password-on="click"
                class="auth-input"
              />
            </n-form-item>

            <n-form-item path="confirmPassword" label="确认密码">
              <n-input
                v-model:value="formValue.confirmPassword"
                type="password"
                placeholder="再次输入密码"
                show-password-on="click"
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
            @click="handleRegister"
          >
            {{ loading ? '创建中...' : '创建账号' }}
          </n-button>

          <div class="modal-footer">
            <span>已经有账号？</span>
            <button
              type="button"
              class="login-link"
              :disabled="loading"
              @click="handleSwitchToLogin"
            >
              去登录
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import type { FormInst } from 'naive-ui'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'to-login'): void
}>()

const userStore = useUserStore()
const message = useMessage()

const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const formValue = ref({
  username: '',
  password: '',
  confirmPassword: '',
})

const USERNAME_MIN_LEN = 3
const USERNAME_MAX_LEN = 50
const PASSWORD_MIN_LEN = 6
const PASSWORD_MAX_LEN = 50

const rules = {
  username: [
    {
      required: true,
      message: '请输入账号',
      trigger: 'blur',
    },
    {
      min: USERNAME_MIN_LEN,
      max: USERNAME_MAX_LEN,
      message: `账号长度需为 ${USERNAME_MIN_LEN}-${USERNAME_MAX_LEN} 个字符`,
      trigger: ['input', 'blur'],
    },
  ],
  password: [
    {
      required: true,
      message: '请输入密码',
      trigger: 'blur',
    },
    {
      min: PASSWORD_MIN_LEN,
      max: PASSWORD_MAX_LEN,
      message: `密码长度需为 ${PASSWORD_MIN_LEN}-${PASSWORD_MAX_LEN} 个字符`,
      trigger: ['input', 'blur'],
    },
  ],
  confirmPassword: [
    {
      required: true,
      message: '请确认密码',
      trigger: 'blur',
    },
    {
      min: PASSWORD_MIN_LEN,
      max: PASSWORD_MAX_LEN,
      message: `密码长度需为 ${PASSWORD_MIN_LEN}-${PASSWORD_MAX_LEN} 个字符`,
      trigger: ['input', 'blur'],
    },
    {
      validator: (rule: any, value: string) => value === formValue.value.password,
      message: '两次输入的密码不一致',
      trigger: 'blur',
    },
  ],
}

function resetForm() {
  formValue.value.username = ''
  formValue.value.password = ''
  formValue.value.confirmPassword = ''
  formRef.value?.restoreValidation()
}

function handleClose() {
  if (loading.value) return
  emit('update:show', false)
  resetForm()
}

function handleSwitchToLogin() {
  if (loading.value) return
  emit('to-login')
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

async function handleRegister() {
  try {
    await formRef.value?.validate()
    loading.value = true
    await userStore.register(formValue.value.username, formValue.value.password)
    message.success('注册成功，请登录')
    resetForm()
    emit('to-login')
  } catch (error: any) {
    if (Array.isArray(error)) return
    message.error(error?.message || '注册失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.register-modal-overlay {
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

.register-modal-content {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(14px);
  width: 100%;
  max-width: 440px;
  padding: 40px 48px;
  border-radius: 22px;
  box-shadow: 0 24px 40px rgba(34, 45, 76, 0.16);
  position: relative;
  border: 1px solid #d9dfec;
  animation: modalIn 0.5s cubic-bezier(0.16, 1, 0.3, 1);
}

@keyframes modalIn {
  from {
    opacity: 0;
    transform: scale(0.95) translateY(20px);
  }
  to {
    opacity: 1;
    transform: scale(1) translateY(0);
  }
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
  margin-bottom: 24px;
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

.modal-footer {
  margin-top: 24px;
  text-align: center;
  font-size: 15px;
  color: #86868b;
}

.login-link {
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

.login-link:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.login-link:hover {
  text-decoration: underline;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
