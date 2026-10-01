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
            invalid,
            required,
          }"
          :label="t('auth.fields.username')"
          :error="errors.displayName"
          field-id="register-display-name"
          required
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
              :class="[authInputClass, 'pl-10', invalid ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="t('auth.placeholders.username')"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              @blur="handleFieldBlur('displayName')"
              @update:model-value="clearFieldError('displayName')"
            />
          </div>
        </IbFormField>

        <EmailField
          v-model="formValue.email"
          :label="t('auth.fields.email')"
          :error="errors.email"
          field-id="register-email"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          :icon-class="authInputIconClass"
          autocomplete="email"
          :disabled="loading"
          :placeholder="t('auth.placeholders.email')"
          @blur="handleFieldBlur('email')"
          @update:model-value="clearFieldError('email')"
        >
          <template #prefix>
            <UserIcon class="size-4.5" />
          </template>
        </EmailField>

        <PasswordField
          v-model="formValue.password"
          :label="t('auth.fields.password')"
          :error="errors.password"
          field-id="register-password"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          :icon-class="authInputIconClass"
          autocomplete="new-password"
          :disabled="loading"
          :placeholder="t('auth.placeholders.password')"
          @blur="handleFieldBlur('password')"
          @update:model-value="handlePasswordInput"
        >
          <template #prefix>
            <LockIcon class="size-4.5" />
          </template>
        </PasswordField>

        <PasswordField
          v-model="formValue.confirmPassword"
          :label="t('auth.fields.confirmPassword')"
          :error="errors.confirmPassword"
          field-id="register-confirm-password"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          :icon-class="authInputIconClass"
          autocomplete="new-password"
          :disabled="loading"
          :placeholder="t('auth.placeholders.password')"
          @blur="handleFieldBlur('confirmPassword')"
          @update:model-value="clearFieldError('confirmPassword')"
        >
          <template #prefix>
            <ShieldIcon class="size-4.5" />
          </template>
        </PasswordField>

        <Button
          :class="authPrimaryButtonClass"
          type="submit"
          :loading="loading"
          v-bind="submitControlState"
          >{{ loading ? t('auth.buttons.creating') : t('auth.buttons.createAccount') }}</Button
        >

        <p class="m-0 text-center text-ib-caption leading-4 text-[var(--color-ib-auth-subtle)]">
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
        <OtpField
          v-model="formValue.otp"
          :label="t('auth.fields.otp')"
          :error="errors.otp"
          field-id="register-otp"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          :placeholder="t('auth.placeholders.otp')"
          @update:model-value="clearFieldError('otp')"
        />
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
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Lock as LockIcon, Shield as ShieldIcon, User as UserIcon } from '@iconoir/vue'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import { PRIVACY_POLICY_PATH, TERMS_PATH } from '../../../shared/constants/routePaths'
import IbFormField from '../../../shared/components/IbFormField.vue'
import { PASSWORD_LENGTH_LIMIT } from '../../../shared/auth/passwordPolicy'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import { useAuthPanelControls } from './panelControls'
import {
  AUTH_DISPLAY_NAME_LIMIT,
  EmailField,
  OtpField,
  PasswordField,
  emailFormatError,
  lengthRangeError,
  otpFieldError,
  passwordLengthFieldError,
  requiredFieldError,
  useAuthForm,
} from './auth-fields'
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

const { t, userStore, authFeedback } = useAuthPanelControls()

type RegisterField = 'displayName' | 'email' | 'password' | 'confirmPassword' | 'otp'

const registerForm = useAuthForm([
  'displayName',
  'email',
  'password',
  'confirmPassword',
  'otp',
] as const)
const formValue = registerForm.values
const errors = registerForm.errors
const otpPending = ref(false)
const loading = computed(() => registerForm.isSubmitting.value)
const submitControlState = computed(() =>
  resolveInteractiveControlState({ pending: loading.value }),
)
const secondaryControlState = computed(() =>
  resolveInteractiveControlState({ disabled: loading.value }),
)

watch(loading, (busy) => emit('update:busy', busy), { immediate: true })
onBeforeUnmount(() => emit('update:busy', false))

function resolveFieldControlState(invalid: boolean) {
  return resolveInteractiveControlState({
    disabled: loading.value,
    tone: invalid ? 'danger' : 'default',
  })
}

function requiredMessage(label: string) {
  return t('auth.errors.requiredField', { field: label })
}

function usernameLengthMessage() {
  return t('auth.errors.lengthRange', {
    label: t('auth.fields.username'),
    min: AUTH_DISPLAY_NAME_LIMIT.min,
    max: AUTH_DISPLAY_NAME_LIMIT.max,
  })
}

function passwordLengthMessage() {
  return t('auth.errors.passwordLength', {
    min: PASSWORD_LENGTH_LIMIT.min,
    max: PASSWORD_LENGTH_LIMIT.max,
  })
}

function handleFieldBlur(field: RegisterField) {
  if (field === 'displayName') {
    const displayName = formValue.value.displayName.trim()
    if (displayName) {
      errors.value.displayName = lengthRangeError(
        displayName,
        AUTH_DISPLAY_NAME_LIMIT,
        usernameLengthMessage(),
      )
    }
  } else if (field === 'email') {
    const email = formValue.value.email.trim()
    if (email) {
      errors.value.email = emailFormatError(email, t('auth.errors.invalidEmail'))
    }
  } else if (field === 'password') {
    const password = formValue.value.password
    if (password) {
      errors.value.password = passwordLengthFieldError(password, passwordLengthMessage())
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
      requiredFieldError(displayName, requiredMessage(t('auth.fields.username'))) ||
      lengthRangeError(displayName, AUTH_DISPLAY_NAME_LIMIT, usernameLengthMessage())
    return !errors.value.displayName
  }

  if (field === 'password') {
    const password = formValue.value.password
    errors.value.password =
      requiredFieldError(password, requiredMessage(t('auth.fields.password'))) ||
      passwordLengthFieldError(password, passwordLengthMessage())
    return !errors.value.password
  }

  if (field === 'email') {
    const email = formValue.value.email.trim()
    errors.value.email =
      requiredFieldError(email, requiredMessage(t('auth.fields.email'))) ||
      emailFormatError(email, t('auth.errors.invalidEmail'))
    return !errors.value.email
  }

  if (field === 'otp') {
    errors.value.otp = otpFieldError(formValue.value.otp, t('auth.errors.sixDigitCode'))
    return !errors.value.otp
  }

  // The confirmation field only checks presence + mismatch: minimum length
  // is owned by the password field (and the server), so a short-but-matching
  // confirmation must not raise its own length error.
  const confirmPassword = formValue.value.confirmPassword
  errors.value.confirmPassword = requiredFieldError(
    confirmPassword,
    requiredMessage(t('auth.fields.confirmPassword')),
  )

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
  registerForm.clearFieldError(field)
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

  const registered = await registerForm.runAsyncAction(
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
  if (!validateField('otp')) return
  const verified = await registerForm.runAsyncAction(
    () => userStore.verifyRegistrationOTP(formValue.value.email, formValue.value.otp.trim()),
    { feedback: authFeedback.register },
  )
  if (verified) emit('register-success')
}
</script>
