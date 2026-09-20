import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen } from '../../../../test/render'
import AdminSettingsGeneralPage from './AdminSettingsGeneralPage.vue'

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }),
}))

describe('AdminSettingsGeneralPage', () => {
  beforeEach(() => {
    useMswHandlers(
      http.get('/api/admin/settings/branding', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { project_name: 'IdeaBoard Test', logo_url: '' },
        }),
      ),
    )
  })

  it('renders website settings table with branding module and expansion', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminSettingsGeneralPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    expect(screen.getByRole('table', { name: '网站设置列表' })).toBeVisible()

    expect(await screen.findByText('站点设置')).toBeVisible()

    // 点击配置展开行
    const configBtn = screen.getByRole('button', { name: '配置' })
    await user.click(configBtn)

    expect(await screen.findByLabelText('站点名称')).toBeVisible()
    expect(screen.getByLabelText('Logo 地址')).toBeVisible()
  })
})
