<template>
  <div class="relative">
    <div :class="authCompactModalShellClass">
      <div :class="authModalBackdropClass" aria-hidden="true"></div>
      <AuthModalCloseButton :disabled="loading" @click="emit('close')" />

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
          v-slot="{
            ariaInvalid,
            describedbyId,
            fieldId,
            inlineError,
            inlineErrorText,
            invalid,
            required,
          }"
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
              v-bind="resolveFieldControlState(invalid)"
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
          v-slot="{
            ariaInvalid,
            describedbyId,
            fieldId,
            inlineError,
            inlineErrorText,
            invalid,
            required,
          }"
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
                ...resolveFieldControlState(invalid),
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
          v-slot="{
            ariaInvalid,
            describedbyId,
            fieldId,
            inlineError,
            inlineErrorText,
            invalid,
            required,
          }"
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
                ...resolveFieldControlState(invalid),
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
          v-bind="submitControlState"
          fluid
        />
      </form>

      <div :class="['mt-6', authFooterClass]">
        <span>已经有账号？</span>
        <button
          type="button"
          :class="authSwitchButtonClass"
          v-bind="secondaryControlState"
          @click="emit('to-login')"
        >
          去登录
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { Lock as LockIcon, Shield as ShieldIcon, User as UserIcon } from '@iconoir/vue'
import { useUserStore } from '../../../app/stores/user'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import IbFormField from '../../../shared/components/IbFormField.vue'
import { useAuthFeedback } from '../useAuthFeedback'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import {
  AUTH_PASSWORD_LIMIT,
  AUTH_USERNAME_LIMIT,
  getLengthError,
  getRequiredError,
} from './authFormUtils'
import {
  authButtonPt,
  authCompactModalShellClass,
  authErrorMessagePt,
  authFieldClass,
  authFooterClass,
  authInputClass,
  authInputErrorClass,
  authInputIconClass,
  authModalBackdropClass,
  authModalHeaderClass,
  authModalKickerClass,
  authModalSubtitleClass,
  authModalTitleClass,
  authPasswordPt,
  authPrimaryButtonClass,
  authSwitchButtonClass,
} from './authModalUi'

const emit = defineEmits<{
  close: []
  'to-login': []
  'register-success': []
  'update:busy': [busy: boolean]
}>()

const { projectName } = useSiteBranding()
const userStore = useUserStore()
const authFeedback = useAuthFeedback()
const { isPending: loading, runAsyncAction } = useAsyncAction()
const submitControlState = computed(() =>
  resolveInteractiveControlState({ pending: loading.value }),
)
const secondaryControlState = computed(() =>
  resolveInteractiveControlState({ disabled: loading.value }),
)

type RegisterField = 'username' | 'password' | 'confirmPassword'

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

watch(loading, (busy) => emit('update:busy', busy), { immediate: true })
onBeforeUnmount(() => emit('update:busy', false))

function resolveFieldControlState(invalid: boolean) {
  return resolveInteractiveControlState({
    disabled: loading.value,
    tone: invalid ? 'danger' : 'default',
  })
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
  if (errors.value[field]) errors.value[field] = ''
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

async function handleRegister() {
  if (!validateForm()) return

  const registered = await runAsyncAction(
    async () => {
      await userStore.register(formValue.value.username.trim(), formValue.value.password)
      return true
    },
    {
      feedback: authFeedback.register,
    },
  )
  if (registered) emit('register-success')
}
</script>
