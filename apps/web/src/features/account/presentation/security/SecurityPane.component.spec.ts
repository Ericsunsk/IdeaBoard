import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import SecurityPane from './SecurityPane.vue'

const usePasswordChangeMock = vi.hoisted(() => vi.fn())

vi.mock('../../application/security/usePasswordChange', () => ({
  usePasswordChange: usePasswordChangeMock,
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
  })

  it('keeps the user-shell primary action visual contract', () => {
    renderWithUser(SecurityPane)

    const submitButton = screen.getByRole('button', { name: 'Update password' })
    expect(submitButton).not.toHaveClass('p-button')
    expect(submitButton).toHaveClass('bg-ib-creative-accent-hover', 'text-ib-text-inverse')
  })
})
