/* eslint-disable vue/one-component-per-file -- Local stubs isolate the pane's visual contract. */
import PrimeVue from 'primevue/config'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen } from '../../../test/render'
import UserSecurityPane from './UserSecurityPane.vue'

const usePasswordChangeWorkflowMock = vi.hoisted(() => vi.fn())

vi.mock('../usePasswordChangeWorkflow', () => ({
  usePasswordChangeWorkflow: usePasswordChangeWorkflowMock,
}))

const PasswordStub = defineComponent({
  props: { modelValue: { type: String, default: '' } },
  template: '<input :value="modelValue" />',
})

const MessageStub = defineComponent({
  template: '<p><slot /></p>',
})

describe('UserSecurityPane', () => {
  beforeEach(() => {
    usePasswordChangeWorkflowMock.mockReset()
    usePasswordChangeWorkflowMock.mockReturnValue({
      form: ref({ oldPassword: '', newPassword: '', confirmPassword: '' }),
      errors: ref({ oldPassword: '', newPassword: '', confirmPassword: '' }),
      loading: ref(false),
      validateField: vi.fn(),
      clearFieldError: vi.fn(),
      handleNewPasswordInput: vi.fn(),
      submit: vi.fn(),
      isInlineError: vi.fn(() => false),
      shouldShowMessage: vi.fn(() => false),
      getDescribedby: vi.fn(),
      displayErrorMessage: vi.fn((value: string) => value),
    })
  })

  it('keeps the user-shell primary action visual contract', () => {
    renderWithUser(UserSecurityPane, {
      global: {
        plugins: [[PrimeVue, primeVueTestOptions]],
        stubs: { Message: MessageStub, Password: PasswordStub },
      },
    })

    const submitButton = screen.getByRole('button', { name: '确认更新密码' })
    expect(submitButton).not.toHaveClass('p-button')
    expect(submitButton).toHaveClass(
      'bg-ib-creative-accent-hover',
      'text-ib-text-inverse',
    )
  })
})
