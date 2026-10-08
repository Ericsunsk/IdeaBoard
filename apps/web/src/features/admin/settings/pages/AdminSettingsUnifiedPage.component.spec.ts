import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { h } from 'vue'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, within } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import { createOutboundPolicyHandlers, createOutboundPolicyMswState } from '../testing'
import AdminSettingsUnifiedPage from './AdminSettingsUnifiedPage.vue'

vi.mock('../../../../shared/composables/useAppToast')

vi.mock('../../shared/logos/brandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))

vi.mock('../../storage/resource', () => ({
  createAdminStorageResource: () => ({
    fetchProviders: vi.fn().mockResolvedValue([
      {
        provider: 's3-compatible',
        label: 'S3 兼容存储',
        create_allowed: true,
        supports_bucket_info: true,
        fields: [
          { key: 'endpoint', label: 'Endpoint', type: 'text', required: true },
          { key: 'bucket_name', label: 'Bucket 名称', type: 'text', required: true },
        ],
      },
    ]),
    fetchProfiles: vi.fn().mockResolvedValue([]),
    saveProfile: vi.fn(),
    deleteProfile: vi.fn(),
    testConnection: vi.fn(),
    selectProfile: vi.fn(),
  }),
}))

describe('AdminSettingsUnifiedPage', () => {
  beforeEach(() => {
    mockAppToast()
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/branding', () =>
        HttpResponse.json({
          data: { project_name: 'IdeaBoard', logo_url: '' },
        }),
      ),
      http.get('/api/control/v1/platform/settings/support-channels', () =>
        HttpResponse.json({
          data: { channels: [] },
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
            ],
          },
        }),
      ),
      http.get('/api/control/v1/platform/settings/media-upload', () =>
        HttpResponse.json({
          data: { upload_max_concurrent: 4 },
        }),
      ),
      http.get('/api/control/v1/platform/settings/mail', () =>
        HttpResponse.json({
          data: {
            configured: false,
            api_key_configured: false,
            from_name: '',
            from_email: '',
            reply_to: '',
            config_version: 0,
            readiness: 'unconfigured',
            updated_at: null,
            last_tested_at: null,
            last_test_error_code: null,
          },
        }),
      ),
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

  it('renders all 8 sections and on-this-page navigation', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(h('main', [h(AdminSettingsUnifiedPage)]), {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    // 目录栏
    const aside = screen.getByRole('complementary')
    expect(within(aside).getByText('本页内容')).toBeVisible()
    expect(within(aside).getByText('站点设置')).toBeVisible()
    expect(within(aside).getByText('客户支持')).toBeVisible()
    expect(within(aside).getByText('图片库')).toBeVisible()
    expect(within(aside).getByText('存储配置')).toBeVisible()
    expect(within(aside).getByText('上传并发限制')).toBeVisible()
    expect(within(aside).getByText('邮件服务')).toBeVisible()
    expect(within(aside).getByText('注册与限流')).toBeVisible()
    expect(within(aside).getByText('私有地址访问')).toBeVisible()

    // 8 个板块在 main 内容区中均呈现
    expect(screen.getAllByRole('main')).toHaveLength(1)
    const main = screen.getByRole('main')
    const sections = [
      ['站点设置', '站点设置'],
      ['客户支持', '客户支持'],
      ['图片库', '图片库'],
      ['存储配置', '存储配置'],
      ['上传并发限制', '上传并发设置'],
      ['邮件发送服务', '邮件服务'],
      ['注册准入与 API 限流', '注册准入与 API 限流'],
      ['私有地址访问策略', '私有地址访问'],
    ] as const
    await vi.waitFor(() => {
      for (const [name, title] of sections) {
        const section = within(main).getByRole('region', { name })
        expect(within(section).getByText(title)).toBeVisible()
      }
    })
  })

  it('navigates via on-this-page item click', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminSettingsUnifiedPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    const mailNavBtn = screen.getByRole('button', { name: '邮件服务' })
    expect(mailNavBtn).toBeVisible()

    await user.click(mailNavBtn)
    expect(mailNavBtn).toHaveAttribute('aria-current', 'true')
  })

  it('renders search load error with retry and recovers on retry', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/search/providers', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'boom' } }, { status: 500 }),
      ),
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminSettingsUnifiedPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    const retryBtn = await screen.findByRole('button', { name: '重试' })
    expect(retryBtn).toBeVisible()
    expect(screen.getByText('boom')).toBeVisible()

    useMswHandlers(
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
            ],
          },
        }),
      ),
    )
    await user.click(retryBtn)

    await vi.waitFor(() => {
      expect(screen.queryByRole('button', { name: '重试' })).not.toBeInTheDocument()
    })
  })
})
