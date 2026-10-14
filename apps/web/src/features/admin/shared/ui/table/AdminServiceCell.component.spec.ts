import { describe, expect, it } from 'vitest'
import { Search } from '@iconoir/vue'
import { markRaw } from 'vue'
import { renderWithUser, screen } from '../../../../../test/render'
import AdminServiceCell from './AdminServiceCell.vue'

describe('AdminServiceCell', () => {
  it('uses the shared icon and node group for known services', async () => {
    const { container, rerender } = renderWithUser(AdminServiceCell, {
      props: { service: 'render', fallbackSubtitle: 'fallback', fallbackIcon: markRaw(Search) },
    })

    expect(screen.getByText('render')).toBeVisible()
    expect(screen.getByText('Image')).toBeVisible()
    const renderIcon = container.querySelector('svg')?.outerHTML
    expect(renderIcon).toBeTruthy()
    expect(screen.queryByText('fallback')).not.toBeInTheDocument()

    await rerender({ service: 'generation' })
    expect(screen.getByText('generation')).toBeVisible()
    expect(screen.getByText('Prompt')).toBeVisible()
    expect(container.querySelector('svg')).not.toBeNull()
    expect(container.querySelector('svg')?.outerHTML).not.toBe(renderIcon)
  })

  it('keeps unknown services visible with caller-provided fallbacks', () => {
    const { container } = renderWithUser(AdminServiceCell, {
      props: {
        service: 'ai-search:vision',
        fallbackSubtitle: 'ai-search',
        fallbackIcon: markRaw(Search),
      },
    })

    expect(screen.getByText('ai-search:vision')).toBeVisible()
    expect(screen.getByText('ai-search')).toBeVisible()
    expect(container.querySelector('svg')).not.toBeNull()
  })

  it.each(['unknown', 'constructor', '__proto__'])(
    'renders %s without an invented icon or subtitle',
    (service) => {
      const { container } = renderWithUser(AdminServiceCell, {
        props: { service },
      })

      expect(screen.getByText(service)).toBeVisible()
      expect(container.querySelector('svg')).toBeNull()
      expect(container.querySelectorAll('span')).toHaveLength(1)
    },
  )
})
