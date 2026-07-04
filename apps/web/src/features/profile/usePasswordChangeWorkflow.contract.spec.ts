import { describe, expect, it, vi } from 'vitest'
import { USER_LOGIN_PATH } from '../../shared/constants/routePaths'
import { markRequiredError } from '../../shared/utils/formValidation'
import {
  usePasswordChangeWorkflow,
  type PasswordChangeWorkflowDependencies,
} from './usePasswordChangeWorkflow'

describe('usePasswordChangeWorkflow', () => {
  it('validates required fields, password length, and confirmation match', () => {
    const workflow = usePasswordChangeWorkflow(createDeps())

    expect(workflow.validateField('oldPassword')).toBe(false)
    expect(workflow.errors.value.oldPassword).toBe(markRequiredError('请输入当前原密码'))
    expect(workflow.isInlineError('oldPassword')).toBe(true)
    expect(workflow.shouldShowMessage('oldPassword')).toBe(false)

    workflow.form.value.newPassword = '12345'
    expect(workflow.validateField('newPassword')).toBe(false)
    expect(workflow.errors.value.newPassword).toBe('密码至少6个字符')
    expect(workflow.shouldShowMessage('newPassword')).toBe(true)
    expect(workflow.getDescribedby('newPassword', 'new-error')).toBe('new-error')

    workflow.form.value.newPassword = '123456'
    workflow.form.value.confirmPassword = 'abcdef'
    expect(workflow.validateField('confirmPassword')).toBe(false)
    expect(workflow.errors.value.confirmPassword).toBe('两次输入密码不一致')
  })

  it('clears new password error and revalidates confirm password when needed', () => {
    const workflow = usePasswordChangeWorkflow(createDeps())

    workflow.form.value.newPassword = '123456'
    workflow.form.value.confirmPassword = 'abcdef'
    workflow.errors.value.newPassword = '密码至少6个字符'
    workflow.errors.value.confirmPassword = '两次输入密码不一致'

    workflow.form.value.newPassword = 'abcdef'
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
    const workflow = usePasswordChangeWorkflow(deps)
    workflow.form.value = {
      oldPassword: 'old-pass',
      newPassword: 'new-pass',
      confirmPassword: 'new-pass',
    }

    await workflow.submit()

    expect(deps.userStore.updatePassword).toHaveBeenCalledWith('old-pass', 'new-pass')
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
    const workflow = usePasswordChangeWorkflow(deps)
    workflow.form.value = {
      oldPassword: 'old-pass',
      newPassword: 'new-pass',
      confirmPassword: 'new-pass',
    }

    await workflow.submit()

    expect(capturedErrors).toEqual(['密码已更新，但退出登录失败，请手动退出后重新登录'])
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
    const workflow = usePasswordChangeWorkflow(deps)
    workflow.form.value = {
      oldPassword: 'old-pass',
      newPassword: 'new-pass',
      confirmPassword: 'new-pass',
    }

    await workflow.submit()

    expect(deps.profileFeedback.passwordChange.error).toHaveBeenCalledWith(redirectError)
    expect(deps.profileFeedback.passwordChange.success).not.toHaveBeenCalled()
  })
})

type TestDeps = PasswordChangeWorkflowDependencies
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
