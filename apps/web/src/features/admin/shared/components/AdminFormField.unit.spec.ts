import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../test/render'
import { defineComponent } from 'vue'
import AdminFormField from './AdminFormField.vue'

describe('AdminFormField', () => {
  it('renders label and hint when there is no error', () => {
    render(AdminFormField, {
      props: {
        label: 'API Key',
        hint: '请输入有效的密钥',
      },
    })
    expect(screen.getByText('API Key')).toBeInTheDocument()
    expect(screen.getByText('请输入有效的密钥')).toBeInTheDocument()
  })

  it('renders visible error message and replaces hint when error is present', () => {
    render(AdminFormField, {
      props: {
        label: 'API Key',
        hint: '请输入有效的密钥',
        error: 'API Key 不能为空',
      },
    })
    expect(screen.getByText('API Key')).toBeInTheDocument()
    const errorEl = screen.getByText('API Key 不能为空')
    expect(errorEl).toBeInTheDocument()
    expect(errorEl).not.toHaveClass('sr-only')
    expect(screen.queryByText('请输入有效的密钥')).not.toBeInTheDocument()
  })

  it('associates the visible error without replacing the input placeholder', () => {
    render(
      defineComponent({
        components: { AdminFormField },
        template: `
          <AdminFormField
            v-slot="{ describedbyId, invalid }"
            label="API Key"
            hint="请输入有效的密钥"
            error="API Key 不能为空"
          >
            <input
              aria-label="API Key input"
              placeholder="输入 API Key"
              :aria-describedby="describedbyId"
              :aria-invalid="invalid"
            />
          </AdminFormField>
        `,
      }),
    )

    const input = screen.getByLabelText('API Key input')
    const error = screen.getByText('API Key 不能为空')
    expect(input).toHaveAttribute('placeholder', '输入 API Key')
    expect(input).toHaveAttribute('aria-describedby', error.id)
    expect(input).toHaveAttribute('aria-invalid', 'true')
  })

  it('shows required asterisk when required is true', () => {
    render(AdminFormField, {
      props: {
        label: '端点名称',
        required: true,
      },
    })
    expect(screen.getByText('*')).toBeInTheDocument()
  })
})
