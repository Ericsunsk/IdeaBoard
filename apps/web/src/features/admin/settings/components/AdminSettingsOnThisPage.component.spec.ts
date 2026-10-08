import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import AdminSettingsOnThisPage from './AdminSettingsOnThisPage.vue'

describe('AdminSettingsOnThisPage', () => {
  const items = [
    { id: 'section-branding', title: '站点设置' },
    { id: 'section-support', title: '客户支持' },
    { id: 'section-images', title: '图片库' },
  ]

  it('scrolls to target offset and emits select when clicked', async () => {
    const scrollToMock = vi.fn()
    vi.spyOn(window, 'scrollTo').mockImplementation(scrollToMock)
    try {
      const targetEl = document.createElement('div')
      targetEl.id = 'section-support'
      // tabindex="-1" 由页面侧负责添加，此处模拟该契约以验证程序化聚焦
      targetEl.tabIndex = -1
      document.body.appendChild(targetEl)

      const onSelect = vi.fn()
      const { user } = renderWithUser(AdminSettingsOnThisPage, {
        props: { items, activeId: 'section-branding', onSelect },
      })

      await user.click(screen.getByRole('button', { name: '客户支持' }))

      expect(onSelect).toHaveBeenCalledWith('section-support')
      expect(scrollToMock).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' })
      expect(targetEl).toHaveFocus()

      document.body.removeChild(targetEl)
    } finally {
      vi.restoreAllMocks()
    }
  })

  it('does not throw when the target section is missing', async () => {
    const scrollToMock = vi.fn()
    vi.spyOn(window, 'scrollTo').mockImplementation(scrollToMock)
    try {
      const onSelect = vi.fn()
      const { user } = renderWithUser(AdminSettingsOnThisPage, {
        props: { items, activeId: 'section-branding', onSelect },
      })

      await user.click(screen.getByRole('button', { name: '客户支持' }))

      expect(onSelect).toHaveBeenCalledWith('section-support')
      expect(scrollToMock).not.toHaveBeenCalled()
    } finally {
      vi.restoreAllMocks()
    }
  })
})
