import { defineComponent } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '../../../../test/render'
import { fireEvent } from '@testing-library/vue'
import CanvasNodeContextMenu from './CanvasNodeContextMenu.vue'
import { NodeToolPopover } from '../nodes/primitives'

const TransitionStub = defineComponent({
  props: {
    name: { type: String, default: '' },
    css: { type: Boolean, default: true },
  },
  template: `
    <div
      data-testid="canvas-transient-transition"
      :data-transition-name="name"
      :data-transition-css="String(css)"
    >
      <slot />
    </div>
  `,
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Canvas transient overlay motion', () => {
  it('keeps context-menu pointer semantics while disabling CSS motion when reduced', () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )
    const onCreate = vi.fn()
    render(CanvasNodeContextMenu, {
      props: {
        visible: true,
        x: 24,
        y: 36,
        items: [{ key: 'prompt', label: '提示词' }],
        onCreate,
      },
      global: { stubs: { transition: TransitionStub } },
    })

    expect(screen.getByTestId('canvas-transient-transition')).toHaveAttribute(
      'data-transition-name',
      'popover-motion',
    )
    expect(screen.getByTestId('canvas-transient-transition')).toHaveAttribute(
      'data-transition-css',
      'false',
    )

    screen
      .getByRole('menuitem', { name: '提示词' })
      .dispatchEvent(new Event('pointerdown', { bubbles: true, cancelable: true }))
    expect(onCreate).toHaveBeenCalledWith('prompt')
  })

  it('keeps the context menu inside the viewport when opened near an edge', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false })),
    )
    render(CanvasNodeContextMenu, {
      props: {
        visible: true,
        x: 1000,
        y: 760,
        items: [{ key: 'prompt', label: '提示词' }],
        onCreate: vi.fn(),
      },
      global: { stubs: { transition: TransitionStub } },
    })

    await vi.waitFor(() => {
      const menu = document.querySelector('.canvas-node-context-menu')
      expect(menu).toHaveStyle({ left: '856px', top: '714px' })
    })
  })

  it('exposes menu semantics, focuses the first action, and supports keyboard navigation', async () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    trigger.focus()
    const onCreate = vi.fn()
    const { rerender } = render(CanvasNodeContextMenu, {
      props: {
        visible: true,
        x: 24,
        y: 36,
        items: [
          { key: 'prompt', label: '提示词' },
          { key: 'image', label: '图像' },
        ],
        onCreate,
      },
      global: { stubs: { transition: TransitionStub } },
    })

    await waitFor(() => {
      expect(screen.getByRole('menu')).toBeInTheDocument()
      expect(screen.getByRole('menuitem', { name: '提示词' })).toHaveFocus()
    })

    fireEvent.keyDown(screen.getByRole('menuitem', { name: '提示词' }), { key: 'ArrowDown' })
    expect(screen.getByRole('menuitem', { name: '图像' })).toHaveFocus()
    fireEvent.keyDown(screen.getByRole('menuitem', { name: '图像' }), { key: 'Enter' })
    expect(onCreate).toHaveBeenCalledWith('image')

    await rerender({ visible: false })
    expect(trigger).toHaveFocus()
    trigger.remove()
  })

  it('keeps Node tool Popover layout, attrs, and show/hide behavior', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false })),
    )
    const { container, rerender } = render(NodeToolPopover, {
      props: { visible: true, layout: 'row' },
      attrs: { role: 'dialog', 'aria-label': '节点工具' },
      slots: { default: '<button type="button">操作</button>' },
      global: { stubs: { transition: TransitionStub } },
    })

    expect(screen.getByTestId('canvas-transient-transition')).toHaveAttribute(
      'data-transition-name',
      'popover-motion',
    )
    expect(screen.getByTestId('canvas-transient-transition')).toHaveAttribute(
      'data-transition-css',
      'true',
    )
    expect(screen.getByRole('dialog', { name: '节点工具' })).toBeVisible()
    expect(screen.getByRole('button', { name: '操作' })).toBeVisible()

    await rerender({ visible: false, layout: 'row' })
    expect(container.querySelector('[role="dialog"]')).toBeNull()
  })
})
