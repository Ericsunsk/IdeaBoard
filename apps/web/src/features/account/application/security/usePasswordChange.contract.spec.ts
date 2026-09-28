import { describe, expect, it, vi } from 'vitest'
import { USER_LOGIN_PATH } from '../../../../shared/constants/routePaths'
import { PASSWORD_LENGTH_LIMIT } from '../../../../shared/auth/passwordPolicy'
import { i18n } from '../../../../shared/i18n'
import { markRequiredError } from '../../../../shared/utils/formValidation'
import { usePasswordChange, type PasswordChangeDependencies } from './usePasswordChange'

function passwordLengthMessage() {
  return i18n.global.t('account.security.errors.passwordLength', {
    min: PASSWORD_LENGTH_LIMIT.min,
    max: PASSWORD_LENGTH_LIMIT.max,
  })
}

describe('usePasswordChange', () => {
  it('validates required fields, password length, and confirmation match', () => {
    const workflow = usePasswordChange(createDeps())

    expect(workflow.validateField('oldPassword')).toBe(false)
    expect(workflow.errors.value.oldPassword).toBe(markRequiredError('Enter your current password'))

    workflow.form.value.newPassword = '123456789'
    expect(workflow.validateField('newPassword')).toBe(false)
    expect(workflow.errors.value.newPassword).toBe(passwordLengthMessage())

    workflow.form.value.newPassword = 'x'.repeat(129)
    expect(workflow.validateField('newPassword')).toBe(false)
    expect(workflow.errors.value.newPassword).toBe(passwordLengthMessage())

    workflow.form.value.newPassword = '1234567890'
    workflow.form.value.confirmPassword = 'abcdef'
    expect(workflow.validateField('confirmPassword')).toBe(false)
    expect(workflow.errors.value.confirmPassword).toBe('Passwords do not match')
  })

  it('clears new password error and revalidates confirm password when needed', () => {
    const workflow = usePasswordChange(createDeps())

    workflow.form.value.newPassword = '1234567890'
    workflow.form.value.confirmPassword = 'abcdefghij'
    workflow.errors.value.newPassword = 'Password must be 10-128 characters'
    workflow.errors.value.confirmPassword = 'Passwords do not match'

    workflow.form.value.newPassword = 'abcdefghij'
    workflow.handleNewPasswordInput()

    expect(workflow.errors.value.newPassword).toBe('')
    expect(workflow.errors.value.confirmPassword).toBe('')
  })

  it('updates password, resets form, logs out, redirects, and then shows success', async () => {
    const calls: string[] = []
    const deps = createDeps({
      updatePassword: vi.fn(async () => {
        calls.push('updatePassword')
      }),
      logout: vi.fn(async () => {
        calls.push('logout')
        return true
      }),
      passwordChangeSuccess: vi.fn(() => {
        calls.push('success')
      }),
      replace: vi.fn(async () => {
        calls.push('replace')
      }),
    })
    const workflow = usePasswordChange(deps)
    workflow.form.value = {
      oldPassword: 'old-pass',
      newPassword: 'new-password',
      confirmPassword: 'new-password',
    }

    await workflow.submit()

    expect(deps.userStore.updatePassword).toHaveBeenCalledWith('old-pass', 'new-password')
    expect(workflow.form.value).toEqual({
      oldPassword: '',
      newPassword: '',
      confirmPassword: '',
    })
    expect(deps.profileFeedback.passwordChange.success).toHaveBeenCalledWith(true)
    expect(deps.router.replace).toHaveBeenCalledWith(USER_LOGIN_PATH)
    expect(calls).toEqual(['updatePassword', 'logout', 'replace', 'success'])
  })

  it('uses the logout failure message through password feedback', async () => {
    const capturedErrors: string[] = []
    const deps = createDeps({
      logout: vi.fn(async () => false),
      runAsyncAction: async (action, options = {}) => {
        try {
          const result = await action()
          options.feedback?.success?.(result)
          return result
        } catch (error) {
          options.feedback?.error?.(error)
          return undefined
        }
      },
      passwordChangeError: vi.fn((error) => {
        capturedErrors.push(error instanceof Error ? error.message : String(error))
      }),
    })
    const workflow = usePasswordChange(deps)
    workflow.form.value = {
      oldPassword: 'old-pass',
      newPassword: 'new-password',
      confirmPassword: 'new-password',
    }

    await workflow.submit()

    expect(capturedErrors).toEqual([
      'Your password was updated, but sign-out failed. Sign out manually and sign in again.',
    ])
    expect(deps.router.replace).not.toHaveBeenCalled()
  })

  it('routes redirect failures through password feedback', async () => {
    const redirectError = new Error('redirect failed')
    const deps = createDeps({
      replace: vi.fn(async () => {
        throw redirectError
      }),
      passwordChangeError: vi.fn(),
    })
    const workflow = usePasswordChange(deps)
    workflow.form.value = {
      oldPassword: 'old-pass',
      newPassword: 'new-password',
      confirmPassword: 'new-password',
    }

    await workflow.submit()

    expect(deps.profileFeedback.passwordChange.error).toHaveBeenCalledWith(redirectError)
    expect(deps.profileFeedback.passwordChange.success).not.toHaveBeenCalled()
  })
})

type TestDeps = PasswordChangeDependencies
type TestUserStore = NonNullable<TestDeps['userStore']>
type TestRouter = NonNullable<TestDeps['router']>
type TestProfileFeedback = NonNullable<TestDeps['profileFeedback']>

function createDeps(
  overrides: Partial<{
    updatePassword: TestUserStore['updatePassword']
    logout: TestUserStore['logout']
    passwordChangeSuccess: NonNullable<TestProfileFeedback['passwordChange']['success']>
    passwordChangeError: NonNullable<TestProfileFeedback['passwordChange']['error']>
    replace: TestRouter['replace']
    runAsyncAction: NonNullable<TestDeps['runAsyncAction']>
  }> = {},
): Required<TestDeps> {
  const defaultRunAsyncAction: NonNullable<TestDeps['runAsyncAction']> = async (
    action,
    options = {},
  ) => {
    if (options.loading) options.loading.value = true
    try {
      const result = await action()
      options.feedback?.success?.(result)
      return result
    } catch (error) {
      options.feedback?.error?.(error)
      return undefined
    } finally {
      if (options.loading) options.loading.value = false
    }
  }

  return {
    userStore: {
      updatePassword: overrides.updatePassword ?? vi.fn(async () => undefined),
      logout: overrides.logout ?? vi.fn(async () => true),
    },
    router: {
      replace: overrides.replace ?? vi.fn(async () => undefined),
    },
    profileFeedback: {
      passwordChange: {
        success: overrides.passwordChangeSuccess ?? vi.fn(),
        error: overrides.passwordChangeError ?? vi.fn(),
      },
    },
    runAsyncAction: overrides.runAsyncAction ?? defaultRunAsyncAction,
  }
}
