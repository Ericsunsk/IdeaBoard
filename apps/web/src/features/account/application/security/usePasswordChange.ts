import { ref, type Ref } from 'vue'
import type { Router } from 'vue-router'
import { useRouter } from 'vue-router'
import { useUserStore } from '../../../auth'
import {
  type AsyncActionFeedback,
  useAsyncAction,
} from '../../../../shared/composables/useAsyncAction'
import {
  getPasswordLengthError,
  PASSWORD_LENGTH_LIMIT,
} from '../../../../shared/auth/passwordPolicy'
import { USER_LOGIN_PATH } from '../../../../shared/constants/routePaths'
import { markRequiredError } from '../../../../shared/utils/formValidation'
import { i18n } from '../../../../shared/i18n'
import { type ProfileFeedback, useProfileFeedback } from '../profile/useProfileFeedback'

export type PasswordField = 'oldPassword' | 'newPassword' | 'confirmPassword'
export type PasswordForm = Record<PasswordField, string>

type RunAsyncAction = <T>(
  action: () => T | Promise<T>,
  options?: {
    loading?: Ref<boolean>
    feedback?: AsyncActionFeedback<T>
  },
) => Promise<T | undefined>

type PasswordChangeUserStore = {
  updatePassword: (oldPassword: string, newPassword: string) => Promise<unknown>
  logout: () => Promise<boolean>
}

export type PasswordChangeDependencies = {
  userStore?: PasswordChangeUserStore
  router?: Pick<Router, 'replace'>
  profileFeedback?: Pick<ProfileFeedback, 'passwordChange'>
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

export function usePasswordChange(deps: PasswordChangeDependencies = {}) {
  const userStore = deps.userStore ?? useUserStore()
  const router = deps.router ?? useRouter()
  const profileFeedback = deps.profileFeedback ?? useProfileFeedback()
  const runAsyncAction = deps.runAsyncAction ?? useAsyncAction().runAsyncAction

  const form = ref<PasswordForm>(emptyPasswordForm())
  const errors = ref<Record<PasswordField, string>>(emptyPasswordForm())
  const loading = ref(false)

  function resolveFieldError(field: PasswordField) {
    const value = form.value[field]

    if (field === 'oldPassword' && !value.trim()) {
      return markRequiredError(i18n.global.t('account.security.errors.enterCurrentPassword'))
    }
    if (field === 'newPassword') {
      if (!value.trim()) {
        return markRequiredError(i18n.global.t('account.security.errors.enterNewPassword'))
      }
      return getPasswordLengthError(
        value,
        i18n.global.t('account.security.errors.passwordLength', {
          min: PASSWORD_LENGTH_LIMIT.min,
          max: PASSWORD_LENGTH_LIMIT.max,
        }),
      )
    }
    if (field === 'confirmPassword') {
      if (!value.trim()) {
        return markRequiredError(i18n.global.t('account.security.errors.confirmNewPassword'))
      }
      if (value !== form.value.newPassword) {
        return i18n.global.t('account.security.errors.mismatch')
      }
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
          throw new Error(i18n.global.t('account.security.errors.logoutAfterChangeFailed'))
        }
        await router.replace(USER_LOGIN_PATH)
        return true
      },
      {
        loading,
        feedback: profileFeedback.passwordChange,
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
  }
}
