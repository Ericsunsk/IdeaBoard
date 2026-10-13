import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import NodeToolIconButton from './NodeToolIconButton.vue'

describe('NodeToolIconButton', () => {
  it('forwards surface classes, styles, and pointer handlers to the native button', async () => {
    const onPointerdown = vi.fn()
    const { user } = renderWithUser(NodeToolIconButton, {
      props: { tooltip: '强度' },
      attrs: {
        class: 'image-action-icon-btn',
        style: { '--node-tool-icon-button-size': '32px' },
        onPointerdown,
      },
    })
    const control = screen.getByRole('button', { name: '强度' })

    expect(control).toHaveClass('image-action-icon-btn', 'node-tool-icon-button')
    expect(control.style.getPropertyValue('--node-tool-icon-button-size')).toBe('32px')
    await user.pointer({ target: control, keys: '[MouseLeft>]' })
    expect(onPointerdown).toHaveBeenCalledOnce()
  })

  it('supports pointer and keyboard activation through its accessible name', async () => {
    const onClick = vi.fn()
    const { user } = renderWithUser(NodeToolIconButton, {
      props: { tooltip: '裁剪', onClick },
      slots: { default: '<svg aria-hidden="true" />' },
    })
    const control = screen.getByRole('button', { name: '裁剪' })

    await user.click(control)
    control.focus()
    await user.keyboard('{Enter}')

    expect(control).toHaveFocus()
    expect(onClick).toHaveBeenCalledTimes(2)
  })

  it('exposes selected state without changing the surface-specific visual adapter', () => {
    renderWithUser(NodeToolIconButton, {
      props: { tooltip: '移动', isActive: true },
    })

    const control = screen.getByRole('button', { name: '移动' })
    expect(control).toHaveAttribute('aria-pressed', 'true')
    expect(control).toHaveAttribute('data-control-state', 'selected')
  })

  it('keeps a disabled icon control readable and non-interactive', async () => {
    const onClick = vi.fn()
    const { user } = renderWithUser(NodeToolIconButton, {
      props: { tooltip: '删除', isDisabled: true, onClick },
      attrs: { disabled: false, 'data-control-state': 'idle' },
    })
    const control = screen.getByRole('button', { name: '删除' })

    await user.click(control)
    control.focus()
    await user.keyboard('{Enter}')

    expect(control).toBeDisabled()
    expect(control).toHaveAttribute('data-control-state', 'disabled')
    expect(onClick).not.toHaveBeenCalled()
  })
})
