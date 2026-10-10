import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import SecurityPane from './SecurityPane.vue'

const usePasswordChangeMock = vi.hoisted(() => vi.fn())
const mockUserStore = vi.hoisted(() => ({
  verifyCurrentPassword: vi.fn(),
  requestEmailChangeOTP: vi.fn(),
  changeEmailWithOTP: vi.fn(),
}))
const mockToast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('../../application/security/usePasswordChange', () => ({
  usePasswordChange: usePasswordChangeMock,
}))

vi.mock('../../../auth', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../auth')>()),
  useUserStore: () => mockUserStore,
}))

vi.mock('../../../../core/composables/useAppToast', () => ({
  useAppToast: () => mockToast,
}))

describe('SecurityPane', () => {
  beforeEach(() => {
    usePasswordChangeMock.mockReset()
    usePasswordChangeMock.mockReturnValue({
      form: ref({ oldPassword: '', newPassword: '', confirmPassword: '' }),
      errors: ref({ oldPassword: '', newPassword: '', confirmPassword: '' }),
      loading: ref(false),
      validateField: vi.fn(),
      clearFieldError: vi.fn(),
      handleNewPasswordInput: vi.fn(),
      submit: vi.fn(),
    })
    vi.clearAllMocks()
    mockUserStore.requestEmailChangeOTP.mockResolvedValue(true)
  })

  it('keeps the user-shell primary action visual contract', () => {
    renderWithUser(SecurityPane)

    const submitButton = screen.getByRole('button', { name: 'Update password' })
    expect(submitButton).not.toHaveClass('p-button')
    expect(submitButton).toHaveClass('bg-primary', 'text-primary-foreground')
  })

  it('shows the sending spinner in the new email input suffix instead of the send button', async () => {
    let resolveVerify!: (value: boolean) => void
    mockUserStore.verifyCurrentPassword.mockReturnValueOnce(
      new Promise<boolean>((resolve) => {
        resolveVerify = resolve
      }),
    )
    const { user } = renderWithUser(SecurityPane)

    await user.type(screen.getByLabelText(/Current password \(re-verify\)/), 'correct-pw')
    await user.type(screen.getByLabelText(/New email/), 'new@example.com')
    await user.click(screen.getByRole('button', { name: 'Send verification code' }))

    // 发送中：newEmail 尾部出现可访问的 sending 状态，按钮仅 disabled、自身无 spinner。
    const status = await screen.findByRole('status')
    expect(status).toHaveTextContent('Sending…')
    const spinner = status.querySelector('svg')
    expect(spinner?.getAttribute('aria-hidden')).toBe('true')
    const sendButton = screen.getByRole('button', { name: 'Sending…' })
    expect(sendButton).toBeDisabled()
    expect(sendButton.querySelector('svg')).not.toBeInTheDocument()
    // loading 行为：输入栏标记 aria-busy（不测内部 Spinner 实现）。
    expect(screen.getByLabelText(/New email/)).toHaveAttribute('aria-busy', 'true')

    resolveVerify(true)
    await waitFor(() => {
      expect(mockUserStore.requestEmailChangeOTP).toHaveBeenCalledWith('new@example.com')
    })
    await waitFor(() => {
      expect(screen.getByLabelText(/Email verification code/)).toBeVisible()
    })
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('keeps the dual-input precondition: invalid email never sends', async () => {
    const { user } = renderWithUser(SecurityPane)

    await user.type(screen.getByLabelText(/Current password \(re-verify\)/), 'correct-pw')
    await user.type(screen.getByLabelText(/New email/), 'not-an-email')
    await user.click(screen.getByRole('button', { name: 'Send verification code' }))

    await waitFor(() => {
      expect(screen.getByText('Enter a valid email address')).toBeVisible()
    })
    expect(mockUserStore.verifyCurrentPassword).not.toHaveBeenCalled()
    expect(mockUserStore.requestEmailChangeOTP).not.toHaveBeenCalled()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
})
