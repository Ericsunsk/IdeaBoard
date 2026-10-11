import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import NodeActionButton from './NodeActionButton.vue'
import NodeToolIconButton from './NodeToolIconButton.vue'

describe('canvas icon explicit name precedence', () => {
  it('prefers explicit aria label over tooltip for NodeToolIconButton icon-only rendering', () => {
    renderWithUser(NodeToolIconButton, {
      props: { ariaLabel: 'Explicit action', tooltip: 'Fallback tooltip' },
      slots: { default: '<svg aria-hidden="true" />' },
    })

    const control = screen.getByRole('button', { name: 'Explicit action' })
    expect(control).toHaveAttribute('aria-label', 'Explicit action')
    expect(screen.queryByRole('button', { name: 'Fallback tooltip' })).toBeNull()
    // Icon-only slot survives: no text content, tooltip stays visual-only.
    expect(control.textContent).toContain('Fallback tooltip')
    expect(control.querySelector('.canvas-tool-tooltip')).toHaveAttribute('aria-hidden', 'true')
  })

  it('prefers explicit aria label over tooltip for NodeActionButton icon-only rendering', () => {
    renderWithUser(NodeActionButton, {
      props: { ariaLabel: 'Explicit action', tooltip: 'Fallback tooltip' },
      slots: { default: '<svg aria-hidden="true" />' },
    })

    const control = screen.getByRole('button', { name: 'Explicit action' })
    expect(control).toHaveAttribute('aria-label', 'Explicit action')
    expect(screen.queryByRole('button', { name: 'Fallback tooltip' })).toBeNull()
    expect(control.querySelector('.canvas-tool-tooltip')).toHaveAttribute('aria-hidden', 'true')
  })

  it('falls back to tooltip when no explicit aria label is given', () => {
    renderWithUser(NodeToolIconButton, {
      props: { tooltip: 'Fallback tooltip' },
      slots: { default: '<svg aria-hidden="true" />' },
    })

    const control = screen.getByRole('button', { name: 'Fallback tooltip' })
    expect(control).toHaveAttribute('aria-label', 'Fallback tooltip')
  })
})
