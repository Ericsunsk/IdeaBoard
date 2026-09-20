import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen } from '../../../../test/render'
import AdminSettingsMailPage from './AdminSettingsMailPage.vue'

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }),
}))

describe('AdminSettingsMailPage', () => {
  beforeEach(() => {
    useMswHandlers(
      http.get('/api/admin/settings/mail', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            configured: true,
            from_name: 'IdeaBoard Admin',
            from_email: 'admin@example.com',
            reply_to: 'reply@example.com',
            verified: true,
          },
        }),
      ),
    )
  })

  it('renders mail settings table and expands mail config card', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminSettingsMailPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    expect(screen.getByRole('table', { name: '邮件服务设置列表' })).toBeVisible()

    expect(await screen.findByText('邮件发送')).toBeVisible()

    // 点击配置展开行
    const configBtn = screen.getByRole('button', { name: '配置' })
    await user.click(configBtn)

    expect(await screen.findByLabelText('发件人名称')).toBeVisible()
  })
})
