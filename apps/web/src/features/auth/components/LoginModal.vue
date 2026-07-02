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
    <Transition name="auth-modal" appear>
      <div v-if="show" class="relative">
        <div :class="authModalShellClass">
          <div :class="authModalBackdropClass" aria-hidden="true"></div>
          <AuthModalCloseButton :disabled="loading" @click="handleClose" />

          <div :class="authModalHeaderClass">
            <p :class="authModalKickerClass">WELCOME BACK</p>
            <h2 :class="authModalTitleClass">欢迎回来</h2>
            <p :class="authModalSubtitleClass">登录 {{ projectName }}，继续你的创作</p>
          </div>

          <form class="relative z-[1] flex flex-col gap-5" novalidate @submit.prevent="handleLogin">
            <AuthFormField
              v-slot="{ describedbyId, inlineError, inlineErrorText, invalid }"
              label="账号"
              :error="errors.username"
              error-id="login-username-error"
            >
              <div class="relative w-full">
                <span :class="authInputIconClass">
                  <UserIcon class="size-5" />
                </span>
                <InputText
                  v-model="formValue.username"
                  unstyled
                  autocomplete="username"
                  :class="[authInputClass, 'pl-11', inlineError ? authInputErrorClass : '']"
                  :invalid="invalid"
                  :placeholder="inlineError ? inlineErrorText : '请输入账号'"
                  :aria-describedby="describedbyId"
                  @blur="validateField('username')"
                  @update:model-value="clearFieldError('username')"
                />
              </div>
            </AuthFormField>

            <AuthFormField
              v-slot="{ describedbyId, inlineError, inlineErrorText, invalid }"
              label="密码"
              :error="errors.password"
              error-id="login-password-error"
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
                    autocomplete: 'current-password',
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
            </AuthFormField>

            <Button
              unstyled
              :class="authPrimaryButtonClass"
              :pt="authButtonPt"
              type="submit"
              :label="loading ? '登录中...' : '登录'"
              :loading="loading"
              :disabled="loading"
              fluid
            />
          </form>

          <div :class="['mt-8', authFooterClass]">
            <span>还没有账号？</span>
            <button
              type="button"
              :class="authSwitchButtonClass"
              :disabled="loading"
              @click="handleSwitchToRegister"
            >
              立即注册
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Dialog>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { User as UserIcon, Lock as LockIcon } from '@iconoir/vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { useAuthFeedback } from '../useAuthFeedback'
import AuthFormField from './AuthFormField.vue'
import AuthModalCloseButton from './AuthModalCloseButton.vue'
import { getRequiredError } from './authFormUtils'
import {
  authDialogPt,
  authButtonPt,
  authFooterClass,
  authInputIconClass,
  authInputClass,
  authInputErrorClass,
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

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'to-register'): void
  (e: 'login-success'): void
}>()

const { projectName } = useSiteBranding()

type LoginField = 'username' | 'password'

const userStore = useUserStore()
const authFeedback = useAuthFeedback()
const { runAsyncAction } = useAsyncAction()
const loginFieldLabels: Record<LoginField, string> = {
  username: '账号',
  password: '密码',
}

const loading = ref(false)
const formValue = ref({
  username: '',
  password: '',
})
const errors = ref<Record<LoginField, string>>({
  username: '',
  password: '',
})

function resetValidation() {
  errors.value.username = ''
  errors.value.password = ''
}

function resetForm() {
  formValue.value.username = ''
  formValue.value.password = ''
  resetValidation()
}

function validateField(field: LoginField) {
  const value = formValue.value[field].trim()
  errors.value[field] = getRequiredError(value, loginFieldLabels[field])
  return !errors.value[field]
}

function validateForm() {
  const usernameValid = validateField('username')
  const passwordValid = validateField('password')
  return usernameValid && passwordValid
}

function clearFieldError(field: LoginField) {
  if (errors.value[field]) {
    errors.value[field] = ''
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

function handleSwitchToRegister() {
  if (loading.value) return
  emit('to-register')
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

async function handleLogin() {
  if (!validateForm()) return

  const loggedIn = await runAsyncAction(
    async () => {
      await userStore.login(formValue.value.username.trim(), formValue.value.password)
      return true
    },
    {
      loading,
      feedback: authFeedback.login,
    },
  )
  if (!loggedIn) return
  resetForm()
  emit('login-success')
  emit('update:show', false)
}
</script>

<style scoped>
.auth-modal-enter-active,
.auth-modal-leave-active,
.auth-modal-appear-active {
  transition:
    opacity var(--canvas-motion-dock, 240ms)
      var(--canvas-motion-ease, cubic-bezier(0.2, 0.8, 0.2, 1)),
    transform var(--canvas-motion-dock, 240ms)
      var(--canvas-motion-ease, cubic-bezier(0.2, 0.8, 0.2, 1));
}

.auth-modal-enter-from,
.auth-modal-leave-to,
.auth-modal-appear-from {
  opacity: 0;
  transform: translateY(24px) scale(0.96);
}

.auth-modal-enter-to,
.auth-modal-leave-from,
.auth-modal-appear-to {
  opacity: 1;
  transform: translateY(0) scale(1);
}

@media (prefers-reduced-motion: reduce) {
  .auth-modal-enter-active,
  .auth-modal-leave-active,
  .auth-modal-appear-active {
    transition: none;
  }
}
</style>
