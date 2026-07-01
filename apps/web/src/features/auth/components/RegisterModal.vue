<template>
  <Dialog
    unstyled
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
    <Transition name="auth-modal" appear>
      <div v-if="show" class="relative">
        <div :class="authCompactModalShellClass">
          <div :class="authModalBackdropClass" aria-hidden="true"></div>
          <AuthModalCloseButton :disabled="loading" @click="handleClose" />

          <div :class="authModalHeaderClass">
            <p :class="authModalKickerClass">START MAKING</p>
            <h2 :class="authModalTitleClass">创建账号</h2>
            <p :class="authModalSubtitleClass">创建你的 {{ projectName }} 账号，开始创作</p>
          </div>

          <form
            class="relative z-[1] flex flex-col gap-4"
            novalidate
            @submit.prevent="handleRegister"
          >
            <AuthFormField
              v-slot="{ describedbyId, inlineError, inlineErrorText, invalid }"
              label="账号"
              :error="errors.username"
              error-id="register-username-error"
            >
              <div class="relative w-full">
                <span :class="authInputIconClass">
                  <UserIcon class="size-5" />
                </span>
                <InputText
                  v-model="formValue.username"
                  unstyled
                  autocomplete="username"
                  :class="[authInputClass, 'pl-11', inlineError ? authInputErrorClass : '']"
                  :invalid="invalid"
                  :placeholder="inlineError ? inlineErrorText : '设置你的账号'"
                  :aria-describedby="describedbyId"
                  @blur="validateField('username')"
                  @update:model-value="clearFieldError('username')"
                />
              </div>
            </AuthFormField>

            <AuthFormField
              v-slot="{ describedbyId, inlineError, inlineErrorText, invalid }"
              label="密码"
              :error="errors.password"
              error-id="register-password-error"
            >
              <div class="relative w-full">
                <span :class="authInputIconClass">
                  <LockIcon class="size-5" />
                </span>
                <Password
                  v-model="formValue.password"
                  unstyled
                  autocomplete="new-password"
                  :feedback="false"
                  fluid
                  :pt="authPasswordPt"
                  :input-props="{
                    autocomplete: 'new-password',
                    'aria-describedby': describedbyId,
                  }"
                  :invalid="invalid"
                  :input-class="[authInputClass, inlineError ? authInputErrorClass : '']"
                  :placeholder="inlineError ? inlineErrorText : '设置登录密码'"
                  toggle-mask
                  @blur="validateField('password')"
                  @update:model-value="handlePasswordInput"
                />
              </div>
            </AuthFormField>

            <AuthFormField
              v-slot="{ describedbyId, inlineError, inlineErrorText, invalid }"
              label="确认密码"
              :error="errors.confirmPassword"
              error-id="register-confirm-password-error"
            >
              <div class="relative w-full">
                <span :class="authInputIconClass">
                  <ShieldIcon class="size-5" />
                </span>
                <Password
                  v-model="formValue.confirmPassword"
                  unstyled
                  autocomplete="new-password"
                  :feedback="false"
                  fluid
                  :pt="authPasswordPt"
                  :input-props="{
                    autocomplete: 'new-password',
                    'aria-describedby': describedbyId,
                  }"
                  :invalid="invalid"
                  :input-class="[authInputClass, inlineError ? authInputErrorClass : '']"
                  :placeholder="inlineError ? inlineErrorText : '再次输入密码'"
                  toggle-mask
                  @blur="validateField('confirmPassword')"
                  @update:model-value="clearFieldError('confirmPassword')"
                />
              </div>
            </AuthFormField>

            <Button
              unstyled
              :class="authPrimaryButtonClass"
              :pt="authButtonPt"
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
      </div>
    </Transition>
  </Dialog>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { User as UserIcon, Lock as LockIcon, Shield as ShieldIcon } from '@iconoir/vue'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import AuthFormField from './AuthFormField.vue'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import {
  AUTH_PASSWORD_LIMIT,
  AUTH_USERNAME_LIMIT,
  getLengthError,
  getRequiredError,
} from './authFormUtils'
import {
  authCompactModalShellClass,
  authButtonPt,
  authDialogPt,
  authFooterClass,
  authInputIconClass,
  authInputClass,
  authInputErrorClass,
  authModalBackdropClass,
  authModalHeaderClass,
  authModalKickerClass,
  authModalSubtitleClass,
  authModalTitleClass,
  authPasswordPt,
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

const { projectName } = useSiteBranding()

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

<style scoped>
.auth-modal-enter-active,
.auth-modal-leave-active,
.auth-modal-appear-active {
  transition:
    opacity var(--canvas-motion-dock, 240ms) var(--canvas-motion-ease, cubic-bezier(0.2, 0.8, 0.2, 1)),
    transform var(--canvas-motion-dock, 240ms) var(--canvas-motion-ease, cubic-bezier(0.2, 0.8, 0.2, 1));
}

.auth-modal-enter-from,
.auth-modal-leave-to,
.auth-modal-appear-from {
  opacity: 0;
  transform: translateY(24px) scale(0.96);
}

.auth-modal-enter-to,
.auth-modal-leave-from,
.auth-modal-appear-to {
  opacity: 1;
  transform: translateY(0) scale(1);
}

@media (prefers-reduced-motion: reduce) {
  .auth-modal-enter-active,
  .auth-modal-leave-active,
  .auth-modal-appear-active {
    transition: none;
  }
}
</style>
