import { ref, type Ref } from 'vue'
import type { Router } from 'vue-router'
import { useRouter } from 'vue-router'
import { useUserStore } from '../../app/stores/user'
import { useAsyncAction } from '../../shared/composables/useAsyncAction'
import { useAppToast } from '../../shared/composables/useAppToast'
import { USER_LOGIN_PATH } from '../../shared/constants/routePaths'
import {
  displayErrorMessage,
  isRequiredErrorMessage,
  markRequiredError,
} from '../../shared/utils/formValidation'

export type PasswordField = 'oldPassword' | 'newPassword' | 'confirmPassword'
export type PasswordForm = Record<PasswordField, string>

type RunAsyncAction = <T>(
  action: () => T | Promise<T>,
  options?: {
    loading?: Ref<boolean>
    errorMessage?: string | ((error: unknown) => string)
  },
) => Promise<T | undefined>

type PasswordChangeUserStore = {
  updatePassword: (oldPassword: string, newPassword: string) => Promise<unknown>
  logout: () => Promise<boolean>
}

type PasswordChangeToast = {
  success: (message: string) => void
}

export type PasswordChangeWorkflowDependencies = {
  userStore?: PasswordChangeUserStore
  router?: Pick<Router, 'replace'>
  appToast?: PasswordChangeToast
  runAsyncAction?: RunAsyncAction
}

const passwordFields: PasswordField[] = ['oldPassword', 'newPassword', 'confirmPassword']

function emptyPasswordForm(): PasswordForm {
  return {
    oldPassword: '',
    newPassword: '',
    confirmPassword: '',
  }
}

export function usePasswordChangeWorkflow(deps: PasswordChangeWorkflowDependencies = {}) {
  const userStore = deps.userStore ?? useUserStore()
  const router = deps.router ?? useRouter()
  const appToast = deps.appToast ?? useAppToast()
  const runAsyncAction = deps.runAsyncAction ?? useAsyncAction().runAsyncAction

  const form = ref<PasswordForm>(emptyPasswordForm())
  const errors = ref<Record<PasswordField, string>>(emptyPasswordForm())
  const loading = ref(false)

  function resolveFieldError(field: PasswordField) {
    const value = form.value[field]

    if (field === 'oldPassword' && !value.trim()) {
      return markRequiredError('请输入当前原密码')
    }
    if (field === 'newPassword') {
      if (!value.trim()) return markRequiredError('请输入新密码')
      if (value.length < 6) return '密码至少6个字符'
    }
    if (field === 'confirmPassword') {
      if (!value.trim()) return markRequiredError('请确认新密码')
      if (value !== form.value.newPassword) return '两次输入密码不一致'
    }
    return ''
  }

  function validateField(field: PasswordField) {
    errors.value[field] = resolveFieldError(field)
    return !errors.value[field]
  }

  function validateForm() {
    return passwordFields.map((field) => validateField(field)).every(Boolean)
  }

  function isInlineError(field: PasswordField) {
    return isRequiredErrorMessage(errors.value[field])
  }

  function shouldShowMessage(field: PasswordField) {
    return Boolean(errors.value[field]) && !isInlineError(field)
  }

  function getDescribedby(field: PasswordField, errorId: string) {
    return shouldShowMessage(field) ? errorId : undefined
  }

  function clearFieldError(field: PasswordField) {
    if (errors.value[field]) {
      errors.value[field] = ''
    }
  }

  function handleNewPasswordInput() {
    clearFieldError('newPassword')
    if (form.value.confirmPassword && errors.value.confirmPassword) {
      validateField('confirmPassword')
    }
  }

  function resetForm() {
    form.value = emptyPasswordForm()
    errors.value = emptyPasswordForm()
  }

  async function submit() {
    if (!validateForm()) return

    await runAsyncAction(
      async () => {
        await userStore.updatePassword(form.value.oldPassword, form.value.newPassword)
        resetForm()
        const isLoggedOut = await userStore.logout()
        if (!isLoggedOut) {
          throw new Error('密码已更新，但退出登录失败，请手动退出后重新登录')
        }
        appToast.success('密码更新成功，请重新登录')
        await router.replace(USER_LOGIN_PATH)
      },
      {
        loading,
        errorMessage: (error) =>
          error instanceof Error && error.message ? error.message : '密码更新失败',
      },
    )
  }

  return {
    form,
    errors,
    loading,
    validateField,
    clearFieldError,
    handleNewPasswordInput,
    submit,
    isInlineError,
    shouldShowMessage,
    getDescribedby,
    displayErrorMessage,
  }
}
