import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen } from '../../../../test/render'
import AdminSettingsSecurityPage from './AdminSettingsSecurityPage.vue'

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }),
}))

describe('AdminSettingsSecurityPage', () => {
  beforeEach(() => {
    useMswHandlers(
      http.get('/api/admin/settings/registration', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { enabled: true },
        }),
      ),
      http.get('/api/admin/settings/rate-limit', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { window_seconds: 60, max_requests: 100 },
        }),
      ),
      http.get('/api/admin/settings/outbound-policy', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { allow_hosts: ['models.internal'] },
        }),
      ),
    )
  })

  it('renders security settings table with registration and outbound policy modules', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(AdminSettingsSecurityPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    expect(screen.getByRole('table', { name: '安全与网络设置列表' })).toBeVisible()

    expect(await screen.findByText('注册准入与 API 限流')).toBeVisible()
    expect(await screen.findByText('私有地址访问')).toBeVisible()
  })
})
