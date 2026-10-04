<template>
  <div class="relative">
    <div :class="authModalShellClass">
      <div :class="authModalBackdropClass" aria-hidden="true"></div>
      <AuthModalCloseButton :disabled="loading" @click="emit('close')" />

      <div :class="authModalHeaderClass">
        <p :class="authModalKickerClass">{{ t('auth.kickerWelcomeBack') }}</p>
        <h2 :class="authModalTitleClass">{{ t('auth.signIn') }}</h2>
      </div>

      <form
        v-if="!resetMode"
        class="relative z-[1] flex flex-col gap-3.5"
        novalidate
        @submit.prevent="handleLogin"
      >
        <div
          v-if="enabledSocialProviders.length"
          :class="[
            'grid gap-2.5',
            enabledSocialProviders.length === 1 ? 'grid-cols-1' : 'grid-cols-2',
          ]"
        >
          <Button
            v-for="provider in enabledSocialProviders"
            :key="provider"
            type="button"
            :class="authSocialIconButtonClass"
            :disabled="loading"
            :aria-label="t('auth.socialSignInAria', { provider: getOAuthProviderLabel(provider) })"
            :title="t('auth.socialSignInAria', { provider: getOAuthProviderLabel(provider) })"
            @click="handleSocialLogin(provider)"
          >
            <img
              v-if="providerLogoUrls[provider]"
              :src="providerLogoUrls[provider]"
              class="size-5"
              :class="provider === 'discord' ? 'scale-[1.12]' : ''"
              alt=""
              aria-hidden="true"
            />
            <span v-else class="text-xs font-semibold uppercase">{{ provider.slice(0, 1) }}</span>
          </Button>
        </div>

        <EmailField
          v-model="formValue.identifier"
          :label="t('auth.fields.email')"
          :error="errors.identifier"
          field-id="login-identifier"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          :icon-class="authInputIconClass"
          autocomplete="email"
          :disabled="loading"
          :placeholder="t('auth.placeholders.email')"
          @blur="handleFieldBlur('identifier')"
          @update:model-value="clearFieldError('identifier')"
        >
          <template #prefix>
            <UserIcon class="size-4.5" />
          </template>
        </EmailField>

        <PasswordField
          v-model="formValue.password"
          :label="t('auth.fields.password')"
          :error="errors.password"
          field-id="login-password"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          :icon-class="authInputIconClass"
          autocomplete="current-password"
          :disabled="loading"
          :placeholder="t('auth.placeholders.password')"
          @blur="handleFieldBlur('password')"
          @update:model-value="clearFieldError('password')"
        >
          <template #prefix>
            <LockIcon class="size-4.5" />
          </template>
          <template #header-extra>
            <button
              type="button"
              class="-my-1 inline-flex min-h-8 items-center border-0 bg-transparent px-1 text-ib-caption font-semibold text-primary transition-colors hover:text-primary/90 hover:underline normal-case tracking-normal cursor-pointer disabled:opacity-60"
              :disabled="loading"
              @click="resetMode = true"
            >
              {{ t('auth.forgotPassword') }}
            </button>
          </template>
        </PasswordField>

        <Button
          :class="authPrimaryButtonClass"
          type="submit"
          :loading="loading"
          v-bind="submitControlState"
          >{{ loading ? t('auth.buttons.signingIn') : t('auth.buttons.signIn') }}</Button
        >
      </form>

      <form
        v-else
        class="relative z-[1] flex flex-col gap-3.5"
        novalidate
        @submit.prevent="handleResetPassword"
      >
        <p class="text-xs text-muted-foreground">{{ t('auth.resetPasswordInstructions') }}</p>
        <EmailField
          v-model="resetForm.email"
          :label="t('auth.fields.email')"
          :error="resetErrors.email"
          field-id="reset-email"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          autocomplete="email"
          :placeholder="t('auth.placeholders.email')"
        />
        <Button
          :class="authPrimaryButtonClass"
          type="submit"
          :loading="loading"
          v-bind="submitControlState"
          >{{ loading ? t('auth.buttons.sending') : t('auth.buttons.sendCode') }}</Button
        >
        <button
          type="button"
          class="self-center border-0 bg-transparent p-0 text-xs font-semibold text-primary transition-colors hover:text-primary/90 hover:underline cursor-pointer disabled:opacity-60"
          :disabled="loading"
          @click="resetMode = false"
        >
          {{ t('auth.backToSignIn') }}
        </button>
      </form>

      <form
        v-if="resetMode && resetOtpSent"
        class="relative z-[1] mt-3 flex flex-col gap-3.5"
        novalidate
        @submit.prevent="handleResetPasswordConfirm"
      >
        <OtpField
          v-model="resetForm.otp"
          :label="t('auth.fields.otp')"
          :error="resetErrors.otp"
          field-id="reset-otp"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          :placeholder="t('auth.placeholders.otp')"
        />
        <PasswordField
          v-model="resetForm.password"
          :label="t('auth.fields.newPassword')"
          :error="resetErrors.password"
          field-id="reset-password"
          :field-class="authFieldClass"
          :input-class="authInputClass"
          :input-error-class="authInputErrorClass"
          :placeholder="t('auth.placeholders.newPassword')"
        />
        <Button
          :class="authPrimaryButtonClass"
          type="submit"
          :loading="loading"
          v-bind="submitControlState"
          >{{ loading ? t('auth.buttons.resetting') : t('auth.buttons.resetPassword') }}</Button
        >
      </form>

      <div :class="['mt-5', authFooterClass]">
        <span>{{ t('auth.footer.dontHaveAccount') }}</span>
        <button
          type="button"
          :class="authSwitchButtonClass"
          v-bind="secondaryControlState"
          @click="emit('to-register')"
        >
          Sign up
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import { Lock as LockIcon, User as UserIcon } from '@iconoir/vue'
import googleLogoUrl from '../../../shared/assets/oauth/google.svg'
import discordLogoUrl from '../../../shared/assets/oauth/discord.svg'
import { authTransport } from '../session/transport'
import { PASSWORD_LENGTH_LIMIT } from '../../../shared/auth/passwordPolicy'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import {
  EmailField,
  OtpField,
  PasswordField,
  emailFormatError,
  otpFieldError,
  passwordLengthFieldError,
  requiredFieldError,
  useAuthForm,
} from './auth-fields'
import { useAuthPanelControls } from './panelControls'
import { getOAuthProviderLabel, type OAuthProviderCode } from '../../../shared/auth/oauthProviders'
import {
  authFieldClass,
  authFooterClass,
  authInputClass,
  authInputErrorClass,
  authInputIconClass,
  authModalBackdropClass,
  authModalHeaderClass,
  authModalKickerClass,
  authModalShellClass,
  authModalTitleClass,
  authPrimaryButtonClass,
  authSocialIconButtonClass,
  authSwitchButtonClass,
} from './modalUi'

const props = defineProps<{
  oauthRedirectPath?: string | undefined
}>()

const emit = defineEmits<{
  close: []
  'to-register': []
  'login-success': []
  'update:busy': [busy: boolean]
}>()

const { t, userStore, authFeedback } = useAuthPanelControls()

type LoginField = 'identifier' | 'password'

const loginFieldLabels = computed<Record<LoginField, string>>(() => ({
  identifier: t('auth.fields.email'),
  password: t('auth.fields.password'),
}))
const loginForm = useAuthForm(['identifier', 'password'] as const)
const resetFormState = useAuthForm(['email', 'otp', 'password'] as const)
const formValue = loginForm.values
const errors = loginForm.errors
const resetForm = resetFormState.values
const resetErrors = resetFormState.errors
const loading = computed(() => loginForm.isSubmitting.value || resetFormState.isSubmitting.value)
const submitControlState = computed(() =>
  resolveInteractiveControlState({ pending: loading.value }),
)
const secondaryControlState = computed(() =>
  resolveInteractiveControlState({ disabled: loading.value }),
)
const resetMode = ref(false)
const resetOtpSent = ref(false)
const socialCapabilities = ref<Record<OAuthProviderCode, boolean>>({})
const providerLogoUrls: Record<string, string> = {
  google: googleLogoUrl,
  discord: discordLogoUrl,
}
const enabledSocialProviders = computed(() =>
  Object.keys(socialCapabilities.value).filter((provider) => socialCapabilities.value[provider]),
)

watch(loading, (busy) => emit('update:busy', busy), { immediate: true })
onBeforeUnmount(() => emit('update:busy', false))

onMounted(async () => {
  socialCapabilities.value = (await authTransport.getCapabilities()).social
})

function requiredMessage(label: string) {
  return t('auth.errors.requiredField', { field: label })
}

function handleFieldBlur(field: LoginField) {
  const value = formValue.value[field].trim()
  if (!value) return
  if (field === 'identifier' && value.includes('@')) {
    errors.value.identifier = emailFormatError(value, t('auth.errors.invalidEmail'))
  }
}

function validateField(field: LoginField) {
  const value = formValue.value[field].trim()
  errors.value[field] = requiredFieldError(value, requiredMessage(loginFieldLabels.value[field]))
  return !errors.value[field]
}

function validateForm() {
  const identifierValid = validateField('identifier')
  const passwordValid = validateField('password')
  return identifierValid && passwordValid
}

function clearFieldError(field: LoginField) {
  loginForm.clearFieldError(field)
}

function passwordLengthMessage() {
  return t('auth.errors.passwordLength', {
    min: PASSWORD_LENGTH_LIMIT.min,
    max: PASSWORD_LENGTH_LIMIT.max,
  })
}

async function handleLogin() {
  if (!validateForm()) return

  const loggedIn = await loginForm.runAsyncAction(
    async () => {
      await userStore.login(formValue.value.identifier, formValue.value.password)
      return true
    },
    {
      feedback: authFeedback.login,
    },
  )
  if (loggedIn) emit('login-success')
}

async function handleSocialLogin(provider: OAuthProviderCode) {
  await loginForm.runAsyncAction(
    async () => {
      if (props.oauthRedirectPath) {
        await authTransport.signInWithOAuth(provider, props.oauthRedirectPath)
      } else {
        await authTransport.signInWithOAuth(provider)
      }
      return true
    },
    { feedback: { error: authFeedback.login.error } },
  )
}

async function handleResetPassword() {
  const email = resetForm.value.email.trim()
  resetErrors.value.email = emailFormatError(email, t('auth.errors.invalidEmail'))
  if (resetErrors.value.email) return

  const sent = await resetFormState.runAsyncAction(() => userStore.requestPasswordResetOTP(email), {
    feedback: authFeedback.login,
  })
  if (sent) resetOtpSent.value = true
}

async function handleResetPasswordConfirm() {
  resetErrors.value.otp = otpFieldError(resetForm.value.otp, t('auth.errors.sixDigitCode'))
  resetErrors.value.password = passwordLengthFieldError(
    resetForm.value.password,
    passwordLengthMessage(),
  )
  if (resetErrors.value.otp || resetErrors.value.password) return

  const reset = await resetFormState.runAsyncAction(
    () =>
      userStore.resetPasswordWithOTP(
        resetForm.value.email,
        resetForm.value.otp,
        resetForm.value.password,
      ),
    { feedback: authFeedback.login },
  )
  if (reset) {
    resetMode.value = false
    resetOtpSent.value = false
    resetForm.value = { email: '', otp: '', password: '' }
  }
}
</script>
