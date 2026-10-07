import { describe, expect, it } from 'vitest'
import { h } from 'vue'
import { renderWithUser, screen } from '../../../../test/render'
import EmailField from './EmailField.vue'
import PasswordField from './PasswordField.vue'

describe('authFields prefix and slot forwarding', () => {
  it('renders prefix icon in EmailField when prefix slot is provided', () => {
    renderWithUser(EmailField, {
      props: {
        label: 'Email',
        modelValue: '',
      },
      slots: {
        prefix: () => h('span', { 'data-testid': 'email-prefix-icon' }, '✉️'),
      },
    })

    const prefix = screen.getByTestId('email-prefix-icon')
    expect(prefix).toBeVisible()
    expect(prefix.parentElement).toHaveClass('absolute', 'left-3.5')
    expect(screen.getByRole('textbox', { name: 'Email' })).toBeVisible()
  })

  it('renders prefix icon in PasswordField when prefix slot is provided', () => {
    renderWithUser(PasswordField, {
      props: {
        label: 'Password',
        modelValue: '',
      },
      slots: {
        prefix: () => h('span', { 'data-testid': 'password-prefix-icon' }, '🔒'),
        'header-extra': () => h('button', { type: 'button' }, 'Forgot?'),
      },
    })

    const prefix = screen.getByTestId('password-prefix-icon')
    expect(prefix).toBeVisible()
    expect(prefix.parentElement).toHaveClass('absolute', 'left-3.5')
    expect(screen.getByRole('button', { name: 'Forgot?' })).toBeVisible()
    expect(screen.getByLabelText(/Password/)).toBeVisible()
  })
})
