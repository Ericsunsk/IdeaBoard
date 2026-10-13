import { fireEvent } from '@testing-library/vue'
import { describe, expect, it, vi } from 'vitest'
import { h } from 'vue'
import { render, screen } from '../../test/render'
import ActionButton from './ActionButton.vue'

// Shared action size keeps the migrated controls' dimensions and typography.
const actionChrome = ['h-9', 'rounded-control', 'px-3', 'text-admin-meta']

describe('ActionButton', () => {
  it('applies the shared action chrome at small size by default', () => {
    render(ActionButton, { slots: { default: '保存' } })

    const button = screen.getByRole('button', { name: '保存' })
    for (const token of actionChrome) {
      expect(button.className.split(' ')).toContain(token)
    }
    expect(button).toHaveAttribute('type', 'button')
  })

  it('forwards action props and click behavior', async () => {
    const onClick = vi.fn()
    render(ActionButton, {
      props: {
        variant: 'outline',
        disabled: false,
        loading: false,
        type: 'submit',
        title: '重试提示',
        ariaLabel: '重试',
      },
      attrs: { onClick },
      slots: { default: '重试' },
    })

    const button = screen.getByRole('button', { name: '重试' })
    expect(button).toHaveAttribute('type', 'submit')
    expect(button).toHaveAttribute('title', '重试提示')
    await fireEvent.click(button)
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('forwards disabled and loading states', () => {
    const { rerender } = render(ActionButton, {
      props: { disabled: true },
      slots: { default: '保存' },
    })
    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()

    void rerender({ disabled: false, loading: true })
    expect(screen.getByRole('button', { name: '保存' })).toBeDisabled()
  })

  it('forwards default and icon slots together', () => {
    render(ActionButton, {
      slots: {
        default: '确认上传',
        icon: () => h('span', { 'data-testid': 'action-icon', 'aria-hidden': 'true' }, 'i'),
      },
    })

    const button = screen.getByRole('button', { name: '确认上传' })
    expect(button).toHaveTextContent('确认上传')
    expect(button.querySelector('[data-testid="action-icon"]')).not.toBeNull()
  })

  it('merges caller classes after the shared action chrome', () => {
    render(ActionButton, {
      props: { variant: 'ghost', class: 'w-full font-bold' },
      slots: { default: '进入管理台' },
    })

    const classes = screen.getByRole('button', { name: '进入管理台' }).className.split(' ')
    for (const token of [...actionChrome, 'w-full', 'font-bold']) {
      expect(classes).toContain(token)
    }
  })

  it('allows overriding the default small size', () => {
    render(ActionButton, {
      props: { size: 'default' },
      slots: { default: '进入管理台' },
    })

    const classes = screen.getByRole('button', { name: '进入管理台' }).className.split(' ')
    expect(classes).toContain('text-admin-body')
    expect(classes).not.toContain('text-admin-meta')
  })
})
