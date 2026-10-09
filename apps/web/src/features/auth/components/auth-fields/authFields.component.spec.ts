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

  it('shows the sending spinner with aria-busy when EmailField loading is true', () => {
    renderWithUser(EmailField, {
      props: {
        label: 'Email',
        modelValue: '',
        loading: true,
      },
    })

    const status = screen.getByRole('status')
    expect(status).toHaveTextContent('Sending…')
    expect(status.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
    const input = screen.getByRole('textbox', { name: 'Email' })
    expect(input).toHaveAttribute('aria-busy', 'true')
    expect(input).toHaveClass('pr-10')
  })

  it('supports a custom loading label', () => {
    renderWithUser(EmailField, {
      props: {
        label: 'Email',
        modelValue: '',
        loading: true,
        loadingLabel: 'Fetching…',
      },
    })

    expect(screen.getByRole('status')).toHaveTextContent('Fetching…')
    expect(screen.getByRole('textbox', { name: 'Email' })).toHaveAttribute('aria-busy', 'true')
  })

  it('hides the spinner and aria-busy when EmailField loading is false', () => {
    renderWithUser(EmailField, {
      props: {
        label: 'Email',
        modelValue: '',
        loading: false,
      },
    })

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Email' })).not.toHaveAttribute('aria-busy')
  })
})
