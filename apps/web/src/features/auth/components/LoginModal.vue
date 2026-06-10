<template>
  <Dialog
    :visible="show"
    modal
    :dismissable-mask="!loading"
    :closable="false"
    :close-on-escape="!loading"
    :draggable="false"
    :show-header="false"
    :pt="authDialogPt"
    @update:visible="handleVisibleChange"
  >
    <div v-motion="authModalMotion" :class="authModalShellClass">
      <AuthModalCloseButton :disabled="loading" @click="handleClose" />

      <div class="mb-8 text-center">
        <h2 :class="authModalTitleClass">欢迎回来</h2>
        <p :class="authModalSubtitleClass">登录 IdeaBoard，继续你的创作</p>
      </div>

      <form class="flex flex-col gap-5" novalidate @submit.prevent="handleLogin">
        <AuthFormField
          v-slot="{ describedbyId, invalid }"
          label="账号"
          :error="errors.username"
          error-id="login-username-error"
        >
          <InputText
            v-model="formValue.username"
            autocomplete="username"
            :class="authInputClass"
            :invalid="invalid"
            placeholder="请输入账号"
            :aria-describedby="describedbyId"
            @blur="validateField('username')"
            @update:model-value="clearFieldError('username')"
          />
        </AuthFormField>

        <AuthFormField
          v-slot="{ describedbyId, invalid }"
          label="密码"
          :error="errors.password"
          error-id="login-password-error"
        >
          <Password
            v-model="formValue.password"
            autocomplete="current-password"
            :feedback="false"
            fluid
            :input-props="{
              autocomplete: 'current-password',
              'aria-describedby': describedbyId,
            }"
            :invalid="invalid"
            :input-class="authInputClass"
            placeholder="请输入密码"
            toggle-mask
            @blur="validateField('password')"
            @update:model-value="clearFieldError('password')"
          />
        </AuthFormField>

        <Button
          :class="authPrimaryButtonClass"
          type="submit"
          :label="loading ? '登录中...' : '登录'"
          :loading="loading"
          :disabled="loading"
          fluid
        />
      </form>

      <div :class="['mt-8', authFooterClass]">
        <span>还没有账号？</span>
        <button
          type="button"
          :class="authSwitchButtonClass"
          :disabled="loading"
          @click="handleSwitchToRegister"
        >
          立即注册
        </button>
      </div>
    </div>
  </Dialog>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { useAppToast } from '../../../shared/composables/useAppToast'
import AuthFormField from './AuthFormField.vue'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import { getRequiredError } from './authFormUtils'
import {
  authDialogPt,
  authFooterClass,
  authInputClass,
  authModalMotion,
  authModalShellClass,
  authModalSubtitleClass,
  authModalTitleClass,
  authPrimaryButtonClass,
  authSwitchButtonClass,
} from './authModalUi'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'to-register'): void
  (e: 'login-success'): void
}>()

type LoginField = 'username' | 'password'

const userStore = useUserStore()
const appToast = useAppToast()
const loginFieldLabels: Record<LoginField, string> = {
  username: '账号',
  password: '密码',
}

const loading = ref(false)
const formValue = ref({
  username: '',
  password: '',
})
const errors = ref<Record<LoginField, string>>({
  username: '',
  password: '',
})

function resetValidation() {
  errors.value.username = ''
  errors.value.password = ''
}

function resetForm() {
  formValue.value.username = ''
  formValue.value.password = ''
  resetValidation()
}

function validateField(field: LoginField) {
  const value = formValue.value[field].trim()
  errors.value[field] = getRequiredError(value, loginFieldLabels[field])
  return !errors.value[field]
}

function validateForm() {
  const usernameValid = validateField('username')
  const passwordValid = validateField('password')
  return usernameValid && passwordValid
}

function clearFieldError(field: LoginField) {
  if (errors.value[field]) {
    errors.value[field] = ''
  }
}

function handleClose() {
  if (loading.value) return
  emit('update:show', false)
  resetForm()
}

function handleVisibleChange(visible: boolean) {
  if (!visible) {
    handleClose()
  }
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
  if (!validateForm()) return

  try {
    loading.value = true
    await userStore.login(formValue.value.username.trim(), formValue.value.password)
    appToast.success('登录成功')
    resetForm()
    emit('login-success')
    emit('update:show', false)
  } catch (error: unknown) {
    appToast.error(error instanceof Error && error.message ? error.message : '登录失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>
