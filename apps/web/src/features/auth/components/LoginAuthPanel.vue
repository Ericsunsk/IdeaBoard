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
            unstyled
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
          :error="errors.identifier"
          field-id="login-identifier"
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
              v-model="formValue.identifier"
              v-bind="resolveFieldControlState(invalid)"
              unstyled
              autocomplete="email"
              :required="required"
              :class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.email')"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              @blur="handleFieldBlur('identifier')"
              @update:model-value="clearFieldError('identifier')"
            />
          </div>
        </IbFormField>

        <IbFormField
          :label="t('auth.fields.password')"
          :error="errors.password"
          field-id="login-password"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <template #header-extra>
            <button
              type="button"
              class="border-0 bg-transparent p-0 text-[11px] font-semibold text-[var(--color-ib-auth-link)] transition-colors hover:text-[var(--color-ib-auth-link-hover)] hover:underline normal-case tracking-normal cursor-pointer disabled:opacity-60"
              :disabled="loading"
              @click="resetMode = true"
            >
              {{ t('auth.forgotPassword') }}
            </button>
          </template>
          <template
            #default="{
              ariaInvalid,
              describedbyId,
              fieldId,
              inlineError,
              inlineErrorText,
              invalid,
              required,
            }"
          >
            <div class="relative w-full">
              <span :class="authInputIconClass">
                <LockIcon class="size-4.5" />
              </span>
              <Password
                v-model="formValue.password"
                unstyled
                autocomplete="current-password"
                :feedback="false"
                fluid
                :pt="authPasswordPt"
                :input-props="{
                  ...resolveFieldControlState(invalid),
                  id: fieldId,
                  autocomplete: 'current-password',
                  required,
                  'aria-invalid': ariaInvalid,
                  'aria-describedby': describedbyId,
                  'aria-expanded': undefined,
                  'aria-haspopup': undefined,
                }"
                :invalid="invalid"
                :input-class="[authInputClass, 'pl-10', inlineError ? authInputErrorClass : '']"
                :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.password')"
                toggle-mask
                @blur="handleFieldBlur('password')"
                @update:model-value="clearFieldError('password')"
              />
            </div>
          </template>
        </IbFormField>

        <Button
          unstyled
          :class="authPrimaryButtonClass"
          :pt="authButtonPt"
          type="submit"
          :label="loading ? t('auth.buttons.signingIn') : t('auth.buttons.signIn')"
          :loading="loading"
          v-bind="submitControlState"
          fluid
        />
      </form>

      <form
        v-else
        class="relative z-[1] flex flex-col gap-3.5"
        novalidate
        @submit.prevent="handleResetPassword"
      >
        <p class="text-xs text-ib-text-subtle">{{ t('auth.resetPasswordInstructions') }}</p>
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
          :error="resetErrors.email"
          field-id="reset-email"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <InputText
            :id="fieldId"
            v-model="resetForm.email"
            unstyled
            autocomplete="email"
            :required="required"
            :class="[authInputClass, inlineError ? authInputErrorClass : '']"
            :invalid="invalid"
            :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.email')"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
          />
        </IbFormField>
        <Button
          unstyled
          :class="authPrimaryButtonClass"
          :pt="authButtonPt"
          type="submit"
          :label="loading ? t('auth.buttons.sending') : t('auth.buttons.sendCode')"
          :loading="loading"
          v-bind="submitControlState"
          fluid
        />
        <button
          type="button"
          class="self-center border-0 bg-transparent p-0 text-xs font-semibold text-ib-creative-accent hover:underline"
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
          :error="resetErrors.otp"
          field-id="reset-otp"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <InputText
            :id="fieldId"
            v-model="resetForm.otp"
            unstyled
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="6"
            :required="required"
            :class="[authInputClass, inlineError ? authInputErrorClass : '']"
            :invalid="invalid"
            :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.otp')"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
          />
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
          :label="t('auth.fields.newPassword')"
          :error="resetErrors.password"
          field-id="reset-password"
          required
          inline-required
          :field-class="authFieldClass"
          :message-pt="authErrorMessagePt"
        >
          <Password
            v-model="resetForm.password"
            unstyled
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
            :placeholder="inlineError ? inlineErrorText : t('auth.placeholders.newPassword')"
            toggle-mask
          />
        </IbFormField>
        <Button
          unstyled
          :class="authPrimaryButtonClass"
          :pt="authButtonPt"
          type="submit"
          :label="loading ? t('auth.buttons.resetting') : t('auth.buttons.resetPassword')"
          :loading="loading"
          v-bind="submitControlState"
          fluid
        />
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
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { Lock as LockIcon, User as UserIcon } from '@iconoir/vue'
import googleLogoUrl from '../../../shared/assets/oauth/google.svg'
import discordLogoUrl from '../../../shared/assets/oauth/discord.svg'
import { authTransport } from '../session/transport'
import { getPasswordLengthError } from '../../../shared/auth/passwordPolicy'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import IbFormField from '../../../shared/components/IbFormField.vue'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import { getRequiredError } from './formUtils'
import { useAuthPanelControls } from './panelControls'
import { getOAuthProviderLabel, type OAuthProviderCode } from '../../../shared/auth/oauthProviders'
import {
  authButtonPt,
  authErrorMessagePt,
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
  authPasswordPt,
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

const {
  t,
  userStore,
  authFeedback,
  loading,
  runAsyncAction,
  submitControlState,
  secondaryControlState,
} = useAuthPanelControls()

type LoginField = 'identifier' | 'password'

const loginFieldLabels = computed<Record<LoginField, string>>(() => ({
  identifier: t('auth.fields.email'),
  password: t('auth.fields.password'),
}))
const formValue = ref({
  identifier: '',
  password: '',
})
const errors = ref<Record<LoginField, string>>({
  identifier: '',
  password: '',
})
const resetMode = ref(false)
const resetOtpSent = ref(false)
const resetForm = ref({ email: '', otp: '', password: '' })
const resetErrors = ref({ email: '', otp: '', password: '' })
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

function resolveFieldControlState(invalid: boolean) {
  return resolveInteractiveControlState({
    disabled: loading.value,
    tone: invalid ? 'danger' : 'default',
  })
}

function handleFieldBlur(field: LoginField) {
  const value = formValue.value[field].trim()
  if (!value) return
  if (field === 'identifier' && (!value.includes('@') || value.endsWith('@'))) {
    errors.value.identifier = t('auth.errors.invalidEmail')
  }
}

function validateField(field: LoginField) {
  const value = formValue.value[field].trim()
  errors.value[field] = getRequiredError(value, loginFieldLabels.value[field])
  return !errors.value[field]
}

function validateForm() {
  const identifierValid = validateField('identifier')
  const passwordValid = validateField('password')
  return identifierValid && passwordValid
}

function clearFieldError(field: LoginField) {
  if (errors.value[field]) errors.value[field] = ''
}

async function handleLogin() {
  if (!validateForm()) return

  const loggedIn = await runAsyncAction(
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
  await runAsyncAction(
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
  resetErrors.value.email = !email || !email.includes('@') ? t('auth.errors.invalidEmail') : ''
  if (resetErrors.value.email) return

  const sent = await runAsyncAction(() => userStore.requestPasswordResetOTP(email), {
    feedback: authFeedback.login,
  })
  if (sent) resetOtpSent.value = true
}

async function handleResetPasswordConfirm() {
  resetErrors.value.otp = /^\d{6}$/.test(resetForm.value.otp.trim())
    ? ''
    : t('auth.errors.sixDigitCode')
  resetErrors.value.password = getPasswordLengthError(resetForm.value.password)
  if (resetErrors.value.otp || resetErrors.value.password) return

  const reset = await runAsyncAction(
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
