import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import AdminSettingsOnThisPage from './AdminSettingsOnThisPage.vue'

describe('AdminSettingsOnThisPage', () => {
  const items = [
    { id: 'section-branding', title: '站点设置' },
    { id: 'section-support', title: '客服渠道' },
    { id: 'section-images', title: '图片库' },
  ]

  it('renders all navigation items with title', () => {
    renderWithUser(AdminSettingsOnThisPage, {
      props: { items, activeId: 'section-branding' },
    })

    expect(screen.getByText('本页内容')).toBeVisible()
    expect(screen.getByText('站点设置')).toBeVisible()
    expect(screen.getByText('客服渠道')).toBeVisible()
    expect(screen.getByText('图片库')).toBeVisible()

    const activeBtn = screen.getByRole('button', { name: '站点设置' })
    expect(activeBtn).toHaveAttribute('aria-current', 'true')
  })

  it('scrolls into view and emits select when clicked', async () => {
    const scrollIntoViewMock = vi.fn()
    const targetEl = document.createElement('div')
    targetEl.id = 'section-support'
    targetEl.scrollIntoView = scrollIntoViewMock
    document.body.appendChild(targetEl)

    const onSelect = vi.fn()
    const { user } = renderWithUser(AdminSettingsOnThisPage, {
      props: { items, activeId: 'section-branding', onSelect },
    })

    await user.click(screen.getByRole('button', { name: '客服渠道' }))

    expect(onSelect).toHaveBeenCalledWith('section-support')
    expect(scrollIntoViewMock).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' })

    document.body.removeChild(targetEl)
  })
})
