import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import { createOutboundPolicyHandlers, createOutboundPolicyMswState } from '../testing'
import AdminSettingsSecurityPage from './AdminSettingsSecurityPage.vue'

vi.mock('../../../../shared/composables/useAppToast')

describe('AdminSettingsSecurityPage', () => {
  beforeEach(() => {
    mockAppToast()
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/registration', () =>
        HttpResponse.json({
          data: { enabled: true },
        }),
      ),
      http.get('/api/control/v1/platform/settings/rate-limit', () =>
        HttpResponse.json({
          data: { window_seconds: 60, max_requests: 100 },
        }),
      ),
      ...createOutboundPolicyHandlers(createOutboundPolicyMswState()),
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
