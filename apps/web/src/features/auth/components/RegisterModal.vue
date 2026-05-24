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
          class="auth-modal-content register-modal-content"
        >
          <AuthModalCloseButton :disabled="loading" @click="handleClose" />

          <div class="auth-modal-header register-modal-header">
            <h2 class="auth-modal-title">创建账号</h2>
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

          <div class="auth-modal-footer register-modal-footer">
            <span>已经有账号？</span>
            <button
              type="button"
              class="auth-modal-switch-link"
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
import { useAppToast } from '../../../shared/composables/useAppToast'
import type { FormInst } from 'naive-ui'
import AuthModalCloseButton from './AuthModalCloseButton.vue'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'to-login'): void
}>()

const userStore = useUserStore()
const appToast = useAppToast()

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
    appToast.success('注册成功，请登录')
    resetForm()
    emit('to-login')
  } catch (error: any) {
    if (Array.isArray(error)) return
    appToast.error(error?.message || '注册失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped src="./auth-modal.css"></style>

<style scoped>
.register-modal-content {
  padding: 40px 48px;
}

.register-modal-header {
  margin-bottom: 24px;
}

.register-modal-footer {
  margin-top: 24px;
}
</style>
