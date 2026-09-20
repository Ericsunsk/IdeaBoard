<template>
  <div class="relative">
    <div :class="authCompactModalShellClass">
      <div :class="authModalBackdropClass" aria-hidden="true"></div>
      <AuthModalCloseButton :disabled="loading" @click="emit('close')" />

      <div :class="authModalHeaderClass">
        <p :class="authModalKickerClass">{{ t('auth.kickerStartMaking') }}</p>
        <h2 :class="authModalTitleClass">{{ t('auth.createAccount') }}</h2>
      </div>

      <form
        v-if="!otpPending"
        class="relative z-[1] flex flex-col gap-3.5"
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
          :label="t('auth.fields.username')"
          :error="errors.displayName"
          field-id="register-display-name"
          required
          inline-required
          :field-class="authFieldClass"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass">
              <UserIcon class="size-4.5" />
            </span>
            <Input
              :id="fieldId"
              v-model="formValue.displayName"
              v-bind="resolveFieldControlState(invalid)"
              autocomplete="name"
              :required="required"
              :class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.username')"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              @blur="handleFieldBlur('displayName')"
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
          :label="t('auth.fields.email')"
          :error="errors.email"
          field-id="register-email"
          required
          inline-required
          :field-class="authFieldClass"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass"><UserIcon class="size-4.5" /></span>
            <Input
              :id="fieldId"
              v-model="formValue.email"
              v-bind="resolveFieldControlState(invalid)"
              autocomplete="email"
              :required="required"
              :class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.email')"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              @blur="handleFieldBlur('email')"
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
          :label="t('auth.fields.password')"
          :error="errors.password"
          field-id="register-password"
          required
          inline-required
          :field-class="authFieldClass"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass">
              <LockIcon class="size-4.5" />
            </span>
            <PasswordInput
              :id="fieldId"
              v-model="formValue.password"
              autocomplete="new-password"
              :required="required"
              :disabled="loading"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              :invalid="invalid"
              :input-class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.password')"
              @blur="handleFieldBlur('password')"
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
          :label="t('auth.fields.confirmPassword')"
          :error="errors.confirmPassword"
          field-id="register-confirm-password"
          required
          inline-required
          :field-class="authFieldClass"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass">
              <ShieldIcon class="size-4.5" />
            </span>
            <PasswordInput
              :id="fieldId"
              v-model="formValue.confirmPassword"
              autocomplete="new-password"
              :required="required"
              :disabled="loading"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              :invalid="invalid"
              :input-class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.password')"
              @blur="handleFieldBlur('confirmPassword')"
              @update:model-value="clearFieldError('confirmPassword')"
            />
          </div>
        </IbFormField>

        <Button
          :class="authPrimaryButtonClass"
          type="submit"
          :loading="loading"
          v-bind="submitControlState"
          >{{ loading ? t('auth.buttons.creating') : t('auth.buttons.createAccount') }}</Button
        >

        <p class="m-0 text-center text-[11px] leading-4 text-[var(--color-ib-auth-subtle)]">
          {{ t('auth.legal.agreePrefix') }}
          <RouterLink
            :to="TERMS_PATH"
            target="_blank"
            class="rounded-ib-control font-semibold text-[var(--color-ib-auth-link)] no-underline focus-visible:outline-none focus-visible:shadow-ib-auth-focus"
          >
            Terms
          </RouterLink>
          &
          <RouterLink
            :to="PRIVACY_POLICY_PATH"
            target="_blank"
            class="rounded-ib-control font-semibold text-[var(--color-ib-auth-link)] no-underline focus-visible:outline-none focus-visible:shadow-ib-auth-focus"
          >
            Privacy
          </RouterLink>
          .
        </p>
      </form>

      <form
        v-else
        class="relative z-[1] flex flex-col gap-3.5"
        novalidate
        @submit.prevent="handleVerifyOTP"
      >
        <p class="text-xs text-slate-600">
          {{ t('auth.otpSentTo', { email: formValue.email }) }}
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
          :label="t('auth.fields.otp')"
          :error="otpError"
          field-id="register-otp"
          required
          inline-required
          :field-class="authFieldClass"
        >
          <Input
            :id="fieldId"
            v-model="otp"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="6"
            :required="required"
            :class="[authInputClass, inlineError ? authInputErrorClass : '']"
            :invalid="invalid"
            :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.otp')"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
            @update:model-value="otpError = ''"
          />
        </IbFormField>
        <Button
          :class="authPrimaryButtonClass"
          type="submit"
          :loading="loading"
          v-bind="submitControlState"
          >{{ loading ? t('auth.buttons.verifying') : t('auth.buttons.verifyAndSignIn') }}</Button
        >
      </form>

      <div :class="['mt-5', authFooterClass]">
        <span>{{ t('auth.footer.alreadyHaveAccount') }}</span>
        <button
          type="button"
          :class="authSwitchButtonClass"
          v-bind="secondaryControlState"
          @click="emit('to-login')"
        >
          {{ t('auth.footer.signIn') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import { Input, PasswordInput } from '@/components/ui/input'
import { Lock as LockIcon, Shield as ShieldIcon, User as UserIcon } from '@iconoir/vue'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import { PRIVACY_POLICY_PATH, TERMS_PATH } from '../../../shared/constants/routePaths'
import IbFormField from '../../../shared/components/IbFormField.vue'
import { getPasswordLengthError } from '../../../shared/auth/passwordPolicy'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import { useAuthPanelControls } from './panelControls'
import {
  AUTH_DISPLAY_NAME_LIMIT,
  getEmailError,
  getLengthError,
  getRequiredError,
} from './formUtils'
import {
  authCompactModalShellClass,
  authFieldClass,
  authFooterClass,
  authInputClass,
  authInputErrorClass,
  authInputIconClass,
  authModalBackdropClass,
  authModalHeaderClass,
  authModalKickerClass,
  authModalTitleClass,
  authPrimaryButtonClass,
  authSwitchButtonClass,
} from './modalUi'

const emit = defineEmits<{
  close: []
  'to-login': []
  'register-success': []
  'update:busy': [busy: boolean]
}>()

const {
  t,
  userStore,
  authFeedback,
  loading,
  runAsyncAction,
  submitControlState,
  secondaryControlState,
} = useAuthPanelControls()

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

function handleFieldBlur(field: RegisterField) {
  if (field === 'displayName') {
    const displayName = formValue.value.displayName.trim()
    if (displayName) {
      errors.value.displayName = getLengthError(
        displayName,
        AUTH_DISPLAY_NAME_LIMIT,
        t('auth.fields.username'),
      )
    }
  } else if (field === 'email') {
    const email = formValue.value.email.trim()
    if (email) {
      errors.value.email = getEmailError(email, t('auth.errors.invalidEmail'))
    }
  } else if (field === 'password') {
    const password = formValue.value.password
    if (password) {
      errors.value.password = getPasswordLengthError(password)
    }
  } else if (field === 'confirmPassword') {
    const confirmPassword = formValue.value.confirmPassword
    if (confirmPassword && confirmPassword !== formValue.value.password) {
      errors.value.confirmPassword = t('auth.errors.passwordMismatch')
    }
  }
}

function validateField(field: RegisterField) {
  if (field === 'displayName') {
    const displayName = formValue.value.displayName.trim()
    errors.value.displayName =
      getRequiredError(displayName, t('auth.fields.username')) ||
      getLengthError(displayName, AUTH_DISPLAY_NAME_LIMIT, t('auth.fields.username'))
    return !errors.value.displayName
  }

  if (field === 'password') {
    const password = formValue.value.password
    errors.value.password =
      getRequiredError(password, t('auth.fields.password')) || getPasswordLengthError(password)
    return !errors.value.password
  }

  if (field === 'email') {
    const email = formValue.value.email.trim()
    errors.value.email =
      getRequiredError(email, t('auth.fields.email')) ||
      getEmailError(email, t('auth.errors.invalidEmail'))
    return !errors.value.email
  }

  const confirmPassword = formValue.value.confirmPassword
  errors.value.confirmPassword =
    getRequiredError(confirmPassword, t('auth.fields.confirmPassword')) ||
    getPasswordLengthError(confirmPassword)

  if (!errors.value.confirmPassword && confirmPassword !== formValue.value.password) {
    errors.value.confirmPassword = t('auth.errors.passwordMismatch')
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
    otpError.value = t('auth.errors.sixDigitCode')
    return
  }
  const verified = await runAsyncAction(
    () => userStore.verifyRegistrationOTP(formValue.value.email, code),
    { feedback: authFeedback.register },
  )
  if (verified) emit('register-success')
}
</script>
