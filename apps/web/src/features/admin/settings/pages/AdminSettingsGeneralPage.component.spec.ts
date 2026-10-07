import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import AdminSettingsGeneralPage from './AdminSettingsGeneralPage.vue'

vi.mock('../../../../shared/composables/useAppToast')

describe('AdminSettingsGeneralPage', () => {
  beforeEach(() => {
    mockAppToast()
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/support-channels', () =>
        HttpResponse.json({ data: { channels: [] } }),
      ),
      http.get('/api/control/v1/platform/settings/branding', () =>
        HttpResponse.json({
          data: { project_name: 'IdeaBoard Test', logo_url: '' },
        }),
      ),
      http.get('/api/control/v1/platform/search/providers', () =>
        HttpResponse.json({
          data: {
            providers: [
              {
                provider: 'unsplash',
                api_key_configured: true,
                enabled: false,
                test_state: 'passed',
              },
              {
                provider: 'pexels',
                api_key_configured: false,
                enabled: false,
                test_state: 'never',
              },
            ],
          },
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

    // 点击配置展开行（首行为站点设置）
    const configBtn = screen.getAllByRole('button', { name: '配置' })[0]!
    await user.click(configBtn)

    expect(await screen.findByLabelText('站点名称')).toBeVisible()
    expect(screen.getByLabelText('Logo 地址')).toBeVisible()
  })

  it('renders image search providers inside website settings', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(AdminSettingsGeneralPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    expect(await screen.findByText('Unsplash 图片源')).toBeVisible()
    expect(screen.getByText('Pexels 图片源')).toBeVisible()
  })

  it('renders support settings and expands its empty editor', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminSettingsGeneralPage, {
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })
    expect(await screen.findByText('0 个已启用渠道')).toBeVisible()
    expect(screen.getByText('共 0 个渠道')).toBeVisible()
    await user.click(screen.getAllByRole('button', { name: '配置' })[1]!)
    expect(await screen.findByText('暂无客服渠道')).toBeVisible()
  })
})
