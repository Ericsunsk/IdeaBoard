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
    <div v-motion="authModalMotion" :class="authCompactModalShellClass">
      <AuthModalCloseButton :disabled="loading" @click="handleClose" />

      <div class="mb-6 text-center">
        <h2 :class="authModalTitleClass">创建账号</h2>
        <p :class="authModalSubtitleClass">创建你的 IdeaBoard 账号，开始创作</p>
      </div>

      <form class="flex flex-col gap-4" novalidate @submit.prevent="handleRegister">
        <label :class="authFieldClass">
          <span>账号</span>
          <InputText
            v-model="formValue.username"
            autocomplete="username"
            :class="authInputClass"
            :invalid="Boolean(errors.username)"
            placeholder="设置你的账号"
            aria-describedby="register-username-error"
            @blur="validateField('username')"
            @update:model-value="clearFieldError('username')"
          />
          <Message
            v-if="errors.username"
            id="register-username-error"
            severity="error"
            size="small"
            :closable="false"
          >
            {{ errors.username }}
          </Message>
        </label>

        <label :class="authFieldClass">
          <span>密码</span>
          <Password
            v-model="formValue.password"
            autocomplete="new-password"
            :feedback="false"
            fluid
            :input-props="{
              autocomplete: 'new-password',
              'aria-describedby': 'register-password-error',
            }"
            :invalid="Boolean(errors.password)"
            :input-class="authInputClass"
            placeholder="设置登录密码"
            toggle-mask
            @blur="validateField('password')"
            @update:model-value="handlePasswordInput"
          />
          <Message
            v-if="errors.password"
            id="register-password-error"
            severity="error"
            size="small"
            :closable="false"
          >
            {{ errors.password }}
          </Message>
        </label>

        <label :class="authFieldClass">
          <span>确认密码</span>
          <Password
            v-model="formValue.confirmPassword"
            autocomplete="new-password"
            :feedback="false"
            fluid
            :input-props="{
              autocomplete: 'new-password',
              'aria-describedby': 'register-confirm-password-error',
            }"
            :invalid="Boolean(errors.confirmPassword)"
            :input-class="authInputClass"
            placeholder="再次输入密码"
            toggle-mask
            @blur="validateField('confirmPassword')"
            @update:model-value="clearFieldError('confirmPassword')"
          />
          <Message
            v-if="errors.confirmPassword"
            id="register-confirm-password-error"
            severity="error"
            size="small"
            :closable="false"
          >
            {{ errors.confirmPassword }}
          </Message>
        </label>

        <Button
          :class="authPrimaryButtonClass"
          type="submit"
          :label="loading ? '创建中...' : '创建账号'"
          :loading="loading"
          :disabled="loading"
          fluid
        />
      </form>

      <div :class="['mt-6', authFooterClass]">
        <span>已经有账号？</span>
        <button
          type="button"
          :class="authSwitchButtonClass"
          :disabled="loading"
          @click="handleSwitchToLogin"
        >
          去登录
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
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useAppToast } from '../../../shared/composables/useAppToast'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import {
  AUTH_PASSWORD_LIMIT,
  AUTH_USERNAME_LIMIT,
  getLengthError,
  getRequiredError,
} from './authFormUtils'
import {
  authCompactModalShellClass,
  authDialogPt,
  authFieldClass,
  authFooterClass,
  authInputClass,
  authModalMotion,
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
  (e: 'to-login'): void
  (e: 'register-success'): void
}>()

type RegisterField = 'username' | 'password' | 'confirmPassword'

const userStore = useUserStore()
const appToast = useAppToast()

const loading = ref(false)
const formValue = ref({
  username: '',
  password: '',
  confirmPassword: '',
})
const errors = ref<Record<RegisterField, string>>({
  username: '',
  password: '',
  confirmPassword: '',
})

function resetValidation() {
  errors.value.username = ''
  errors.value.password = ''
  errors.value.confirmPassword = ''
}

function resetForm() {
  formValue.value.username = ''
  formValue.value.password = ''
  formValue.value.confirmPassword = ''
  resetValidation()
}

function validateField(field: RegisterField) {
  if (field === 'username') {
    const username = formValue.value.username.trim()
    errors.value.username =
      getRequiredError(username, '账号') || getLengthError(username, AUTH_USERNAME_LIMIT, '账号')
    return !errors.value.username
  }

  if (field === 'password') {
    const password = formValue.value.password
    errors.value.password =
      getRequiredError(password, '密码') || getLengthError(password, AUTH_PASSWORD_LIMIT, '密码')
    return !errors.value.password
  }

  const confirmPassword = formValue.value.confirmPassword
  errors.value.confirmPassword =
    getRequiredError(confirmPassword, '密码') ||
    getLengthError(confirmPassword, AUTH_PASSWORD_LIMIT, '密码')

  if (!errors.value.confirmPassword && confirmPassword !== formValue.value.password) {
    errors.value.confirmPassword = '两次输入的密码不一致'
  }
  return !errors.value.confirmPassword
}

function validateForm() {
  const usernameValid = validateField('username')
  const passwordValid = validateField('password')
  const confirmPasswordValid = validateField('confirmPassword')
  return usernameValid && passwordValid && confirmPasswordValid
}

function clearFieldError(field: RegisterField) {
  if (errors.value[field]) {
    errors.value[field] = ''
  }
}

function handlePasswordInput() {
  clearFieldError('password')
  if (
    errors.value.confirmPassword &&
    formValue.value.confirmPassword === formValue.value.password
  ) {
    errors.value.confirmPassword = ''
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
  if (!validateForm()) return

  try {
    loading.value = true
    await userStore.register(formValue.value.username.trim(), formValue.value.password)
    appToast.success('注册成功')
    resetForm()
    emit('register-success')
    emit('update:show', false)
  } catch (error: unknown) {
    appToast.error(error instanceof Error && error.message ? error.message : '注册失败，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>
