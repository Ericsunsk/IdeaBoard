<template>
  <div class="relative">
    <div :class="authCompactModalShellClass">
      <div :class="authModalBackdropClass" aria-hidden="true"></div>
      <AuthModalCloseButton :disabled="loading" @click="emit('close')" />

      <div :class="authModalHeaderClass">
        <p :class="authModalKickerClass">START MAKING</p>
        <h2 :class="authModalTitleClass">Create account</h2>
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
          label="Username"
          :error="errors.displayName"
          field-id="register-display-name"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass">
              <UserIcon class="size-4.5" />
            </span>
            <InputText
              :id="fieldId"
              v-model="formValue.displayName"
              v-bind="resolveFieldControlState(invalid)"
              unstyled
              autocomplete="name"
              :required="required"
              :class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : 'e.g. alex'"
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
          label="Email"
          :error="errors.email"
          field-id="register-email"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass"><UserIcon class="size-4.5" /></span>
            <InputText
              :id="fieldId"
              v-model="formValue.email"
              v-bind="resolveFieldControlState(invalid)"
              unstyled
              autocomplete="email"
              :required="required"
              :class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : 'you@example.com'"
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
          label="Password"
          :error="errors.password"
          field-id="register-password"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass">
              <LockIcon class="size-4.5" />
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
              :input-class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :placeholder="inlineError ? inlineErrorText : '••••••••'"
              toggle-mask
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
          label="Confirm Password"
          :error="errors.confirmPassword"
          field-id="register-confirm-password"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <div class="relative w-full">
            <span :class="authInputIconClass">
              <ShieldIcon class="size-4.5" />
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
              :input-class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :placeholder="inlineError ? inlineErrorText : '••••••••'"
              toggle-mask
              @blur="handleFieldBlur('confirmPassword')"
              @update:model-value="clearFieldError('confirmPassword')"
            />
          </div>
        </IbFormField>

        <Button
          unstyled
          :class="authPrimaryButtonClass"
          :pt="authButtonPt"
          type="submit"
          :label="loading ? 'Creating...' : 'Create account'"
          :loading="loading"
          v-bind="submitControlState"
          fluid
        />

        <p class="m-0 text-center text-[11px] leading-4 text-[var(--color-ib-auth-subtle)]">
          By signing up, you agree to our
          <RouterLink
            :to="TERMS_PATH"
            target="_blank"
            class="rounded-[var(--radius-ib-control)] font-semibold text-[var(--color-ib-auth-link)] no-underline focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-auth-focus)]"
          >
            Terms
          </RouterLink>
          &
          <RouterLink
            :to="PRIVACY_POLICY_PATH"
            target="_blank"
            class="rounded-[var(--radius-ib-control)] font-semibold text-[var(--color-ib-auth-link)] no-underline focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-auth-focus)]"
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
          A verification code was sent to {{ formValue.email }}. Enter it below to complete registration.
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
          label="Verification Code"
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
            :placeholder="inlineError ? inlineErrorText : 'Enter 6-digit code'"
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
          :label="loading ? 'Verifying...' : 'Verify & Sign in'"
          :loading="loading"
          v-bind="submitControlState"
          fluid
        />
      </form>

      <div :class="['mt-5', authFooterClass]">
        <span>Already have an account?</span>
        <button
          type="button"
          :class="authSwitchButtonClass"
          v-bind="secondaryControlState"
          @click="emit('to-login')"
        >
          Sign in
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
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import { PRIVACY_POLICY_PATH, TERMS_PATH } from '../../../shared/constants/routePaths'
import IbFormField from '../../../shared/components/IbFormField.vue'
import { getPasswordLengthError } from '../../../shared/auth/passwordPolicy'
import { useAuthFeedback } from '../useAuthFeedback'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import {
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

function handleFieldBlur(field: RegisterField) {
  if (field === 'displayName') {
    const displayName = formValue.value.displayName.trim()
    if (displayName) {
      errors.value.displayName = getLengthError(displayName, AUTH_DISPLAY_NAME_LIMIT, 'Username')
    }
  } else if (field === 'email') {
    const email = formValue.value.email.trim()
    if (email && (!email.includes('@') || email.endsWith('@'))) {
      errors.value.email = 'Please enter a valid email address'
    }
  } else if (field === 'password') {
    const password = formValue.value.password
    if (password) {
      errors.value.password = getPasswordLengthError(password)
    }
  } else if (field === 'confirmPassword') {
    const confirmPassword = formValue.value.confirmPassword
    if (confirmPassword && confirmPassword !== formValue.value.password) {
      errors.value.confirmPassword = 'Passwords do not match'
    }
  }
}

function validateField(field: RegisterField) {
  if (field === 'displayName') {
    const displayName = formValue.value.displayName.trim()
    errors.value.displayName =
      getRequiredError(displayName, 'Username') ||
      getLengthError(displayName, AUTH_DISPLAY_NAME_LIMIT, 'Username')
    return !errors.value.displayName
  }

  if (field === 'password') {
    const password = formValue.value.password
    errors.value.password = getRequiredError(password, 'Password') || getPasswordLengthError(password)
    return !errors.value.password
  }

  if (field === 'email') {
    const email = formValue.value.email.trim()
    errors.value.email = getRequiredError(email, 'Email')
    if (!errors.value.email && (!email.includes('@') || email.endsWith('@'))) {
      errors.value.email = 'Please enter a valid email address'
    }
    return !errors.value.email
  }

  const confirmPassword = formValue.value.confirmPassword
  errors.value.confirmPassword =
    getRequiredError(confirmPassword, 'Confirm Password') || getPasswordLengthError(confirmPassword)

  if (!errors.value.confirmPassword && confirmPassword !== formValue.value.password) {
    errors.value.confirmPassword = 'Passwords do not match'
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
    otpError.value = 'Please enter a 6-digit code'
    return
  }
  const verified = await runAsyncAction(
    () => userStore.verifyRegistrationOTP(formValue.value.email, code),
    { feedback: authFeedback.register },
  )
  if (verified) emit('register-success')
}
</script>
