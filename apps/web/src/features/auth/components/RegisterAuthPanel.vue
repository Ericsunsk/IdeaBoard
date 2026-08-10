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
        v-if="!otpPending"
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
          label="显示名称"
          :error="errors.displayName"
          field-id="register-display-name"
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
              v-model="formValue.displayName"
              v-bind="resolveFieldControlState(invalid)"
              unstyled
              autocomplete="name"
              :required="required"
              :class="[authInputClass, 'pl-11', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : '输入你的显示名称'"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              @blur="validateField('displayName')"
              @update:model-value="clearFieldError('displayName')"
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
          label="Email"
          :error="errors.email"
          field-id="register-email"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass"><UserIcon class="size-5" /></span>
            <InputText
              :id="fieldId"
              v-model="formValue.email"
              v-bind="resolveFieldControlState(invalid)"
              unstyled
              autocomplete="email"
              :required="required"
              :class="[authInputClass, 'pl-11', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : '输入你的 Email'"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              @blur="validateField('email')"
              @update:model-value="clearFieldError('email')"
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

        <p class="m-0 text-center text-[12px] leading-5 text-[var(--color-ib-auth-subtle)]">
          创建账号即表示你已年满 18 岁。请先阅读
          <RouterLink
            :to="TERMS_PATH"
            target="_blank"
            class="rounded-[var(--radius-ib-control)] font-semibold text-[var(--color-ib-auth-link)] no-underline focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-auth-focus)]"
          >
            Terms
          </RouterLink>
          和
          <RouterLink
            :to="PRIVACY_POLICY_PATH"
            target="_blank"
            class="rounded-[var(--radius-ib-control)] font-semibold text-[var(--color-ib-auth-link)] no-underline focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-auth-focus)]"
          >
            Privacy Policy
          </RouterLink>
          。
        </p>
      </form>

      <form
        v-else
        class="relative z-[1] flex flex-col gap-4"
        novalidate
        @submit.prevent="handleVerifyOTP"
      >
        <p class="text-sm text-slate-600">
          验证码已发送到 {{ formValue.email }}，请输入 6 位验证码完成注册。
        </p>
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
          label="Email 验证码"
          :error="otpError"
          field-id="register-otp"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <InputText
            :id="fieldId"
            v-model="otp"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="6"
            :required="required"
            :class="[authInputClass, inlineError ? authInputErrorClass : '']"
            :invalid="invalid"
            :placeholder="inlineError ? inlineErrorText : '输入 6 位验证码'"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
            @update:model-value="otpError = ''"
          />
        </IbFormField>
        <Button
          unstyled
          :class="authPrimaryButtonClass"
          :pt="authButtonPt"
          type="submit"
          :label="loading ? '验证中...' : '验证并登录'"
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
import { useUserStore } from '../session/authSession'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import { PRIVACY_POLICY_PATH, TERMS_PATH } from '../../../shared/constants/routePaths'
import IbFormField from '../../../shared/components/IbFormField.vue'
import { useAuthFeedback } from '../useAuthFeedback'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import {
  AUTH_PASSWORD_LIMIT,
  AUTH_DISPLAY_NAME_LIMIT,
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

type RegisterField = 'displayName' | 'email' | 'password' | 'confirmPassword'

const formValue = ref({
  displayName: '',
  email: '',
  password: '',
  confirmPassword: '',
})
const otpPending = ref(false)
const otp = ref('')
const otpError = ref('')
const errors = ref<Record<RegisterField, string>>({
  displayName: '',
  email: '',
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
  if (field === 'displayName') {
    const displayName = formValue.value.displayName.trim()
    errors.value.displayName =
      getRequiredError(displayName, '显示名称') ||
      getLengthError(displayName, AUTH_DISPLAY_NAME_LIMIT, '显示名称')
    return !errors.value.displayName
  }

  if (field === 'password') {
    const password = formValue.value.password
    errors.value.password =
      getRequiredError(password, '密码') || getLengthError(password, AUTH_PASSWORD_LIMIT, '密码')
    return !errors.value.password
  }

  if (field === 'email') {
    const email = formValue.value.email.trim()
    errors.value.email = getRequiredError(email, 'Email')
    if (!errors.value.email && (!email.includes('@') || email.endsWith('@'))) {
      errors.value.email = '请输入有效的 Email'
    }
    return !errors.value.email
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
  const displayNameValid = validateField('displayName')
  const emailValid = validateField('email')
  const passwordValid = validateField('password')
  const confirmPasswordValid = validateField('confirmPassword')
  return displayNameValid && emailValid && passwordValid && confirmPasswordValid
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
      await userStore.register(
        formValue.value.email,
        formValue.value.displayName.trim(),
        formValue.value.password,
      )
      return true
    },
    {
      feedback: authFeedback.register,
    },
  )
  if (registered) {
    otpPending.value = true
  }
}

async function handleVerifyOTP() {
  const code = otp.value.trim()
  if (!/^\d{6}$/.test(code)) {
    otpError.value = '请输入 6 位验证码'
    return
  }
  const verified = await runAsyncAction(
    () => userStore.verifyRegistrationOTP(formValue.value.email, code),
    { feedback: authFeedback.register },
  )
  if (verified) emit('register-success')
}
</script>
