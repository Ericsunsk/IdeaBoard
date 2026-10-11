import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../../test/render'
import NodeActionButton from './NodeActionButton.vue'

describe('NodeActionButton tooltip fallback name', () => {
  it('falls back to tooltip when no explicit aria label is given', () => {
    renderWithUser(NodeActionButton, {
      props: { tooltip: 'Fallback tooltip' },
      slots: { default: '<svg aria-hidden="true" />' },
    })

    const control = screen.getByRole('button', { name: 'Fallback tooltip' })
    expect(control).toHaveAttribute('aria-label', 'Fallback tooltip')
  })
})
