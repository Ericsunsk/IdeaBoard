import { describe, expect, it } from 'vitest'
import { render, screen } from '../../../../test/render'
import AdminStatusPill from './AdminStatusPill.vue'

describe('AdminStatusPill', () => {
  it('renders with label and default styling', () => {
    render(AdminStatusPill, {
      props: {
        label: '运行中',
        variant: 'success',
      },
    })
    expect(screen.getByText('运行中')).toBeInTheDocument()
  })

  it('renders small size and danger variant', () => {
    render(AdminStatusPill, {
      props: {
        label: '已冻结',
        variant: 'danger',
        size: 'small',
      },
    })
    expect(screen.getByText('已冻结')).toBeInTheDocument()
  })
})
