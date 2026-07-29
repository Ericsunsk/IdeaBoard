import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import NodeToolIconButton from './NodeToolIconButton.vue'

describe('NodeToolIconButton', () => {
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
      props: { tooltip: '移动', active: true },
    })

    const control = screen.getByRole('button', { name: '移动' })
    expect(control).toHaveAttribute('aria-pressed', 'true')
    expect(control).toHaveAttribute('data-control-state', 'selected')
  })

  it('keeps a disabled icon control readable and non-interactive', async () => {
    const onClick = vi.fn()
    const { user } = renderWithUser(NodeToolIconButton, {
      props: { tooltip: '删除', disabled: true, onClick },
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
