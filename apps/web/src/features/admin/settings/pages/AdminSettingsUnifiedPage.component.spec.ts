import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { h } from 'vue'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, within } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import { createOutboundPolicyHandlers, createOutboundPolicyMswState } from '../shared/testing'
import { SETTINGS_MESSAGES } from './messages'
import { STORAGE_MESSAGES } from '../../storage/pages/messages'
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
      http.get('/api/control/v1/users/oauth/:provider', ({ params }) => {
        const provider = String(params.provider)
        return HttpResponse.json({
          data: {
            provider,
            enabled: false,
            configured: false,
            available: true,
            client_id: '',
            client_secret_configured: false,
            scopes: [],
            config_version: 0,
            callback_url: `https://example.com/api/auth/callback/${provider}`,
            updated_at: null,
          },
        })
      }),
      http.get('/api/control/v1/payments/providers/stripe', () =>
        HttpResponse.json({
          data: {
            configured: false,
            api_key_configured: false,
            webhook_secret_configured: false,
            charges_enabled: false,
            product_tax_code: '',
            product_ready: false,
            webhook_ready: false,
            purchases_enabled: false,
          },
        }),
      ),
      ...createOutboundPolicyHandlers(createOutboundPolicyMswState()),
    )
  })

  it('renders all 9 sections and on-this-page navigation', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(h('main', [h(AdminSettingsUnifiedPage)]), {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    // 目录栏
    const aside = screen.getByRole('complementary')
    expect(within(aside).getByText('本页内容')).toBeVisible()
    expect(within(aside).getByText('通用')).toBeVisible()
    expect(within(aside).getByText('客户支持')).toBeVisible()
    expect(within(aside).getByText('图片搜索')).toBeVisible()
    expect(within(aside).getByText('存储')).toBeVisible()
    expect(within(aside).getByText('邮件服务')).toBeVisible()
    expect(within(aside).getByText('注册限流')).toBeVisible()
    expect(within(aside).getByText('OAuth')).toBeVisible()
    expect(within(aside).getByText('支付')).toBeVisible()
    expect(within(aside).getByText(SETTINGS_MESSAGES.outboundTitle)).toBeVisible()

    // 9 个板块在 main 内容区中均呈现（顺序即目录顺序）
    expect(screen.getAllByRole('main')).toHaveLength(1)
    const main = screen.getByRole('main')
    const sections = [
      ['通用', SETTINGS_MESSAGES.brandingTitle],
      ['客户支持', SETTINGS_MESSAGES.supportTitle],
      ['存储', STORAGE_MESSAGES.storageTitle],
      ['图片搜索', SETTINGS_MESSAGES.imageLibraryTitle],
      ['邮件服务', SETTINGS_MESSAGES.mailTitle],
      ['OAuth', SETTINGS_MESSAGES.oauthLoginTitle],
      ['支付', SETTINGS_MESSAGES.paymentTitle],
      ['注册限流', SETTINGS_MESSAGES.registrationTitle],
      ['出网白名单', SETTINGS_MESSAGES.outboundTitle],
    ] as const
    await vi.waitFor(() => {
      for (const [name, title] of sections) {
        const section = within(main).getByRole('region', { name })
        expect(within(section).getByText(title)).toBeVisible()
      }
      const ordered = within(main)
        .getAllByRole('region')
        .filter((region) => !region.parentElement?.closest('section'))
        .map((region) => region.getAttribute('aria-label'))
      expect(ordered).toEqual(sections.map(([name]) => name))
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
