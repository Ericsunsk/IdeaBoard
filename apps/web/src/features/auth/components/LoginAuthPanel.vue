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

      <form class="relative z-[1] flex flex-col gap-5" novalidate @submit.prevent="handleLogin">
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
          label="Email 或 username"
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
              autocomplete="username"
              :required="required"
              :class="[authInputClass, 'pl-11', inlineError ? authInputErrorClass : '']"
              :invalid="invalid"
              :placeholder="inlineError ? inlineErrorText : '请输入 Email 或 username'"
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
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { Lock as LockIcon, User as UserIcon } from '@iconoir/vue'
import { useUserStore } from '../../../app/stores/user'
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
  identifier: 'Email 或 username',
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

watch(loading, (busy) => emit('update:busy', busy), { immediate: true })
onBeforeUnmount(() => emit('update:busy', false))

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
</script>
