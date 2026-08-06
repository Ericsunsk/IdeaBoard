<template>
  <div class="relative">
    <div :class="authModalShellClass">
      <div :class="authModalBackdropClass" aria-hidden="true"></div>
      <AuthModalCloseButton :disabled="loading" @click="emit('close')" />

      <div :class="authModalHeaderClass">
        <p :class="authModalKickerClass">WELCOME BACK</p>
        <h2 :class="authModalTitleClass">欢迎回来</h2>
        <p :class="authModalSubtitleClass">登录 {{ projectName }}，继续你的创作</p>
      </div>

      <form
        v-if="!resetMode"
        class="relative z-[1] flex flex-col gap-5"
        novalidate
        @submit.prevent="handleLogin"
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
          label="Email"
          :error="errors.identifier"
          field-id="login-identifier"
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
              v-model="formValue.identifier"
              v-bind="resolveFieldControlState(invalid)"
              unstyled
              autocomplete="email"
              :required="required"
              :class="[authInputClass, 'pl-11', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : '请输入 Email'"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedbyId"
              @blur="validateField('identifier')"
              @update:model-value="clearFieldError('identifier')"
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
          field-id="login-password"
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
              }"
              :invalid="invalid"
              :input-class="[authInputClass, inlineError ? authInputErrorClass : '']"
              :placeholder="inlineError ? inlineErrorText : '请输入密码'"
              toggle-mask
              @blur="validateField('password')"
              @update:model-value="clearFieldError('password')"
            />
          </div>
        </IbFormField>

        <Button
          unstyled
          :class="authPrimaryButtonClass"
          :pt="authButtonPt"
          type="submit"
          :label="loading ? '登录中...' : '登录'"
          :loading="loading"
          v-bind="submitControlState"
          fluid
        />

        <div v-if="googleEnabled" class="flex items-center gap-3">
          <div class="h-px flex-1 bg-ib-border-soft" aria-hidden="true"></div>
          <span class="text-xs text-ib-text-subtle">或</span>
          <div class="h-px flex-1 bg-ib-border-soft" aria-hidden="true"></div>
        </div>
        <Button
          v-if="googleEnabled"
          unstyled
          type="button"
          label="使用 Google 登录"
          :class="authSecondaryButtonClass"
          :disabled="loading"
          @click="handleGoogleLogin"
        />
        <button
          type="button"
          class="self-end border-0 bg-transparent p-0 text-xs font-semibold text-ib-creative-accent hover:underline"
          :disabled="loading"
          @click="resetMode = true"
        >
          忘记密码？
        </button>
      </form>

      <form v-else class="relative z-[1] flex flex-col gap-5" novalidate @submit.prevent="handleResetPassword">
        <p class="text-sm text-ib-text-subtle">输入注册 Email，我们会发送一次性验证码。</p>
        <IbFormField
          v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
          label="Email"
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
            :placeholder="inlineError ? inlineErrorText : '输入注册 Email'"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
          />
        </IbFormField>
        <Button
          unstyled
          :class="authPrimaryButtonClass"
          :pt="authButtonPt"
          type="submit"
          :label="loading ? '发送中...' : '发送验证码'"
          :loading="loading"
          v-bind="submitControlState"
          fluid
        />
        <button
          type="button"
          class="self-center border-0 bg-transparent p-0 text-sm font-semibold text-ib-creative-accent hover:underline"
          :disabled="loading"
          @click="resetMode = false"
        >
          返回登录
        </button>
      </form>

      <form v-if="resetMode && resetOtpSent" class="relative z-[1] mt-4 flex flex-col gap-5" novalidate @submit.prevent="handleResetPasswordConfirm">
        <IbFormField
          v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
          label="Email 验证码"
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
            :placeholder="inlineError ? inlineErrorText : '输入 6 位验证码'"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
          />
        </IbFormField>
        <IbFormField
          v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
          label="新密码"
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
            :input-props="{ id: fieldId, autocomplete: 'new-password', required, 'aria-invalid': ariaInvalid, 'aria-describedby': describedbyId }"
            :invalid="invalid"
            :input-class="[authInputClass, inlineError ? authInputErrorClass : '']"
            :placeholder="inlineError ? inlineErrorText : '不少于 10 位字符'"
            toggle-mask
          />
        </IbFormField>
        <Button
          unstyled
          :class="authPrimaryButtonClass"
          :pt="authButtonPt"
          type="submit"
          :label="loading ? '更新中...' : '重置密码'"
          :loading="loading"
          v-bind="submitControlState"
          fluid
        />
      </form>

      <div :class="['mt-8', authFooterClass]">
        <span>还没有账号？</span>
        <button
          type="button"
          :class="authSwitchButtonClass"
          v-bind="secondaryControlState"
          @click="emit('to-register')"
        >
          立即注册
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
import { useUserStore } from '../session/authSession'
import { authTransport } from '../session/authTransport'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import IbFormField from '../../../shared/components/IbFormField.vue'
import { useAuthFeedback } from '../useAuthFeedback'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import { getRequiredError } from './authFormUtils'
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
  authModalSubtitleClass,
  authModalTitleClass,
  authPasswordPt,
  authPrimaryButtonClass,
  authSecondaryButtonClass,
  authSwitchButtonClass,
} from './authModalUi'

const emit = defineEmits<{
  close: []
  'to-register': []
  'login-success': []
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

type LoginField = 'identifier' | 'password'

const loginFieldLabels: Record<LoginField, string> = {
  identifier: 'Email',
  password: '密码',
}
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
const googleEnabled = ref(false)

watch(loading, (busy) => emit('update:busy', busy), { immediate: true })
onBeforeUnmount(() => emit('update:busy', false))

onMounted(async () => {
  googleEnabled.value = (await authTransport.getCapabilities()).social.google
})

function resolveFieldControlState(invalid: boolean) {
  return resolveInteractiveControlState({
    disabled: loading.value,
    tone: invalid ? 'danger' : 'default',
  })
}

function validateField(field: LoginField) {
  const value = formValue.value[field].trim()
  errors.value[field] = getRequiredError(value, loginFieldLabels[field])
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

async function handleGoogleLogin() {
  await runAsyncAction(
    async () => {
      await authTransport.signInWithGoogle()
      return true
    },
    { feedback: authFeedback.login },
  )
}

async function handleResetPassword() {
  const email = resetForm.value.email.trim()
  resetErrors.value.email = !email || !email.includes('@') ? '请输入有效的 Email' : ''
  if (resetErrors.value.email) return

  const sent = await runAsyncAction(
    () => userStore.requestPasswordResetOTP(email),
    { feedback: authFeedback.login },
  )
  if (sent) resetOtpSent.value = true
}

async function handleResetPasswordConfirm() {
  resetErrors.value.otp = /^\d{6}$/.test(resetForm.value.otp.trim()) ? '' : '请输入 6 位验证码'
  resetErrors.value.password = resetForm.value.password.length >= 10 ? '' : '密码至少 10 个字符'
  if (resetErrors.value.otp || resetErrors.value.password) return

  const reset = await runAsyncAction(
    () => userStore.resetPasswordWithOTP(resetForm.value.email, resetForm.value.otp, resetForm.value.password),
    { feedback: authFeedback.login },
  )
  if (reset) {
    resetMode.value = false
    resetOtpSent.value = false
    resetForm.value = { email: '', otp: '', password: '' }
  }
}
</script>
