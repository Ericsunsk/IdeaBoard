import { defineComponent } from 'vue'
import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../test/render'
import IbFormField from './IbFormField.vue'

const FormHarness = defineComponent({
  components: { IbFormField },
  props: {
    hideConfirmMessage: { type: Boolean, default: false },
  },
  template: `
    <form>
      <IbFormField
        v-slot="field"
        label="账号"
        error="__required__:请输入账号"
        field-id="username"
        required
      >
        <input
          v-bind="{
            id: field.fieldId,
            required: field.required,
            'aria-invalid': field.ariaInvalid,
            'aria-describedby': field.describedbyId,
          }"
        />
      </IbFormField>

      <IbFormField v-slot="field" label="密码" error="密码至少6个字符" field-id="password" required>
        <input
          v-bind="{
            id: field.fieldId,
            required: field.required,
            'aria-invalid': field.ariaInvalid,
            'aria-describedby': field.describedbyId,
          }"
        />
      </IbFormField>

      <IbFormField
        v-slot="field"
        label="确认密码"
        error="两次输入密码不一致"
        field-id="confirm-password"
        :show-message="!hideConfirmMessage"
      >
        <input
          v-bind="{
            id: field.fieldId,
            'aria-invalid': field.ariaInvalid,
            'aria-describedby': field.describedbyId,
          }"
        />
      </IbFormField>
    </form>
  `,
})

describe('IbFormField', () => {
  it('connects labels, required state, and persistent error descriptions', async () => {
    const { user } = renderWithUser(FormHarness)

    const username = screen.getByLabelText(/账号/)
    const password = screen.getByLabelText(/^密码/)

    expect(username).toHaveAttribute('id', 'username')
    expect(username).toBeRequired()
    expect(username).toHaveAttribute('aria-invalid', 'true')
    expect(username).toHaveAttribute('aria-describedby', 'username-error')
    expect(username).toHaveAccessibleDescription('请输入账号')
    expect(screen.getByText('请输入账号')).toBeInTheDocument()

    await user.click(screen.getByText('账号', { selector: 'label' }))
    expect(username).toHaveFocus()

    await user.tab()
    expect(password).toHaveFocus()
  })

  it('gives every error a unique element and screen-reader association', () => {
    renderWithUser(FormHarness)

    const password = screen.getByLabelText(/^密码/)
    const confirmPassword = screen.getByLabelText(/^确认密码/)
    const passwordError = document.getElementById('password-error')
    const confirmPasswordError = document.getElementById('confirm-password-error')

    expect(password).toHaveAttribute('aria-invalid', 'true')
    expect(password).toHaveAccessibleDescription('密码至少6个字符')
    expect(passwordError).not.toBeNull()
    expect(confirmPasswordError).not.toBeNull()
    expect(passwordError).toHaveTextContent('密码至少6个字符')
    expect(confirmPasswordError).toHaveTextContent('两次输入密码不一致')
    expect(password).toHaveAttribute('aria-describedby', 'password-error')
    expect(confirmPassword).toHaveAttribute('aria-describedby', 'confirm-password-error')
  })

  it('keeps a screen-reader error when the visible Message is disabled', () => {
    renderWithUser(FormHarness, {
      props: { hideConfirmMessage: true },
    })

    const input = screen.getByLabelText(/^确认密码/)
    const errorNode = document.getElementById('confirm-password-error')

    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(input).toHaveAttribute('aria-describedby', 'confirm-password-error')
    expect(input).toHaveAccessibleDescription('两次输入密码不一致')
    expect(errorNode).toHaveTextContent('两次输入密码不一致')
    expect(errorNode?.querySelector('[role="alert"]')).toBeNull()
  })
})
