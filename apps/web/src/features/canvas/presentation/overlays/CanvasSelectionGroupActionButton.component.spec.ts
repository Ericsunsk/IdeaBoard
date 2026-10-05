import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { render } from '../../../../test/render'
import CanvasSelectionGroupActionButton from './CanvasSelectionGroupActionButton.vue'

const TransitionStub = defineComponent({
  props: {
    name: { type: String, default: '' },
    css: { type: Boolean, default: true },
  },
  template:
    '<div data-testid="group-action-transition" :data-transition-name="name" :data-transition-css="String(css)"><slot /></div>',
})

describe('CanvasSelectionGroupActionButton', () => {
  it('renders the group action as an icon-only accessible button', () => {
    const onActivate = vi.fn()
    const { container } = render(CanvasSelectionGroupActionButton, {
      props: {
        action: { kind: 'group', label: 'Group', nodeIds: ['prompt-1', 'prompt-2'] },
        position: { x: 420, y: 88 },
        onActivate,
      },
      global: { stubs: { transition: TransitionStub } },
    })

    const button = container.querySelector('button')
    expect(button?.getAttribute('aria-label')).toBe('Group')
    expect(button?.getAttribute('title')).toBe('Group')
    expect(button?.getAttribute('class')).toContain('round-control')
    expect(button?.getAttribute('class')).not.toContain('canvas-chrome-icon-button')
    expect(button?.getAttribute('class')).toContain('canvas-tool-tooltip-trigger')
    expect(button?.style.left).toBe('420px')
    expect(button?.style.top).toBe('88px')
    expect(container.querySelector('.canvas-tool-tooltip')?.textContent).toBe('Group')
    expect(container.querySelector('[data-testid="group-action-transition"]')).toHaveAttribute(
      'data-transition-name',
      'popover-motion',
    )

    button?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onActivate).toHaveBeenCalledOnce()
  })

  it('renders the ungroup action and hides when no action is available', () => {
    const visible = render(CanvasSelectionGroupActionButton, {
      props: {
        action: {
          kind: 'ungroup',
          label: 'Ungroup',
          groupId: 'group-1',
          nodeIds: ['prompt-1', 'prompt-2'],
        },
        position: { x: 270, y: 88 },
      },
    })

    expect(visible.container.querySelector('button')?.getAttribute('aria-label')).toBe('Ungroup')
    expect(visible.container.querySelector('.canvas-tool-tooltip')?.textContent).toBe('Ungroup')
    expect(visible.container.querySelector('button')?.style.left).toBe('270px')
    expect(visible.container.querySelector('button')?.style.top).toBe('88px')

    const hidden = render(CanvasSelectionGroupActionButton, {
      props: {
        action: null,
        position: { x: 270, y: 88 },
      },
    })

    expect(hidden.container.querySelector('button')).toBeNull()
  })

  it('hides when no viewport position is available', () => {
    const { container } = render(CanvasSelectionGroupActionButton, {
      props: {
        action: {
          kind: 'ungroup',
          label: 'Ungroup',
          groupId: 'group-1',
          nodeIds: [],
        },
        position: null,
      },
    })

    expect(container.querySelector('button')).toBeNull()
  })
})
