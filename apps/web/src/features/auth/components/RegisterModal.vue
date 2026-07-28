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
    <template v-if="show">
      <div class="relative">
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
            <IbFormField
              v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
              label="账号"
              :error="errors.username"
              field-id="register-username"
              required
              inline-required
              :field-class="authFieldClass"
              :message-pt="authErrorMessagePt"
            >
              <div class="relative w-full">
                <span :class="authInputIconClass">
                  <UserIcon class="size-5" />
                </span>
                <InputText
                  :id="fieldId"
                  v-model="formValue.username"
                  unstyled
                  autocomplete="username"
                  :required="required"
                  :class="[authInputClass, 'pl-11', inlineError ? authInputErrorClass : '']"
                  :invalid="invalid"
                  :placeholder="inlineError ? inlineErrorText : '设置你的账号'"
                  :aria-invalid="ariaInvalid"
                  :aria-describedby="describedbyId"
                  @blur="validateField('username')"
                  @update:model-value="clearFieldError('username')"
                />
              </div>
            </IbFormField>

            <IbFormField
              v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
              label="密码"
              :error="errors.password"
              field-id="register-password"
              required
              inline-required
              :field-class="authFieldClass"
              :message-pt="authErrorMessagePt"
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
                    id: fieldId,
                    autocomplete: 'new-password',
                    required,
                    'aria-invalid': ariaInvalid,
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
            </IbFormField>

            <IbFormField
              v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
              label="确认密码"
              :error="errors.confirmPassword"
              field-id="register-confirm-password"
              required
              inline-required
              :field-class="authFieldClass"
              :message-pt="authErrorMessagePt"
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
                    id: fieldId,
                    autocomplete: 'new-password',
                    required,
                    'aria-invalid': ariaInvalid,
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
            </IbFormField>

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
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { User as UserIcon, Lock as LockIcon, Shield as ShieldIcon } from '@iconoir/vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { useAuthFeedback } from '../useAuthFeedback'
import IbFormField from '../../../shared/components/IbFormField.vue'
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
  createAuthDialogPt,
  authFooterClass,
  authInputIconClass,
  authInputClass,
  authInputErrorClass,
  authErrorMessagePt,
  authFieldClass,
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

const authDialogPt = createAuthDialogPt()
const { projectName } = useSiteBranding()

type RegisterField = 'username' | 'password' | 'confirmPassword'

const userStore = useUserStore()
const authFeedback = useAuthFeedback()
const { runAsyncAction } = useAsyncAction()

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

  const registered = await runAsyncAction(
    async () => {
      await userStore.register(formValue.value.username.trim(), formValue.value.password)
      return true
    },
    {
      loading,
      feedback: authFeedback.register,
    },
  )
  if (!registered) return
  resetForm()
  emit('register-success')
  emit('update:show', false)
}
</script>
