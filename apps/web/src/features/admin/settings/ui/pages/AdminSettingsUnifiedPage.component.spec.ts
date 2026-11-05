import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { h, nextTick } from 'vue'
import { createDeferred } from '@/test/deferred'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { renderWithUser, screen, waitFor, within } from '@/test/render'
import { mockAppToast } from '@/test/toast'
import { createOutboundPolicyHandlers, createOutboundPolicyMswState } from '../../session/testing'
import { SETTINGS_MESSAGES } from './messages'
import { STORAGE_MESSAGES } from '../cards/storage/messages'
import AdminSettingsUnifiedPage from './AdminSettingsUnifiedPage.vue'
import * as searchProviderResource from '../../session/image-library/searchProviderResource'

vi.mock('../../../../../core/composables/useAppToast')

// The real table intentionally hides background loading once rows exist. Expose
// the page's prop for the stale-finally regression while keeping real controls.
vi.mock('../cards/image-library/AdminImageProvidersTable.vue', async (importOriginal) => {
  const { default: Table } =
    await importOriginal<typeof import('../cards/image-library/AdminImageProvidersTable.vue')>()
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      inheritAttrs: false,
      setup(_, { attrs }) {
        return () =>
          h('div', [
            h('output', { 'aria-label': 'Search refresh loading' }, String(attrs.loading)),
            h(Table, attrs),
          ])
      },
    }),
  }
})

vi.mock('../../../shared/session/brand/brandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))

vi.mock('../../session/storage/resource', () => ({
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
  afterEach(() => vi.restoreAllMocks())

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
      http.get('/api/control/v1/platform/settings/login-lockout', () =>
        HttpResponse.json({
          data: { threshold: 10, duration_seconds: 900 },
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

  it('routes search load failure to toast error without an inline alert', async () => {
    const toast = mockAppToast()
    useMswHandlers(
      http.get('/api/control/v1/platform/search/providers', () =>
        HttpResponse.json({ error: { code: 'internal', message: 'boom' } }, { status: 500 }),
      ),
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(AdminSettingsUnifiedPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    await vi.waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('boom')),
    )
    expect(toast.success).not.toHaveBeenCalled()
    // 失败一律 toast，不设内嵌 Alert/重试；图片搜索板块保留为真相源。
    expect(await screen.findByText('外部图片搜索与素材供应商设置')).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重试' })).not.toBeInTheDocument()
    expect(screen.queryByText(/boom/)).not.toBeInTheDocument()
  })

  it('keeps the latest search refresh when an older change response arrives later', async () => {
    let listCalls = 0
    const olderResponse = createDeferred<Response>()
    const resource = searchProviderResource.createAdminSearchProviderResource()
    const list = vi.spyOn(resource, 'list')
    vi.spyOn(searchProviderResource, 'createAdminSearchProviderResource').mockReturnValue(resource)
    const initial = {
      provider: 'unsplash',
      api_key_configured: true,
      enabled: false,
      test_state: 'passed',
    }
    const latest = { ...initial, enabled: false, test_state: 'never' as const }
    useMswHandlers(
      http.put('/api/control/v1/platform/search/providers/:provider', () =>
        HttpResponse.json({ data: {} }),
      ),
      http.get('/api/control/v1/platform/search/providers', () => {
        listCalls += 1
        if (listCalls === 1) return HttpResponse.json({ data: { providers: [initial] } })
        if (listCalls === 2) {
          return olderResponse.promise
        }
        return HttpResponse.json({ data: { providers: [latest] } })
      }),
    )
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(AdminSettingsUnifiedPage, {
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })

    const toggle = await screen.findByRole('switch', { name: 'Enable Unsplash' })
    const section = within(screen.getByRole('region', { name: '图片搜索' }))
    await user.click(toggle)
    await waitFor(() => expect(listCalls).toBe(2))
    // The key save starts the latest request, whose disabled/unverified response
    // wins first while the earlier enabled response remains unresolved.
    await user.type(screen.getByLabelText('Unsplash API Key'), 'replacement-key')
    await user.click(section.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(listCalls).toBe(3))
    await list.mock.results[2]!.value
    await nextTick()
    expect(section.getByText('Unverified')).toBeVisible()

    olderResponse.resolve(
      HttpResponse.json({ data: { providers: [{ ...initial, enabled: true }] } }),
    )
    await list.mock.results[1]!.value
    await nextTick()
    expect(section.getByText('Unverified')).toBeVisible()
    expect(screen.getByRole('switch', { name: 'Enable Unsplash' })).toHaveAttribute(
      'data-state',
      'unchecked',
    )
  })

  it.each(['before latest', 'after latest'] as const)(
    'ignores an older search refresh failure %s completion',
    async (order) => {
      const toast = mockAppToast()
      const older = createDeferred<Response>()
      const latest = createDeferred<Response>()
      const resource = searchProviderResource.createAdminSearchProviderResource()
      const list = vi.spyOn(resource, 'list')
      vi.spyOn(searchProviderResource, 'createAdminSearchProviderResource').mockReturnValue(
        resource,
      )
      let listCalls = 0
      const initial = {
        provider: 'unsplash',
        api_key_configured: true,
        enabled: false,
        test_state: 'passed',
      }
      useMswHandlers(
        http.put('/api/control/v1/platform/search/providers/:provider', () =>
          HttpResponse.json({ data: {} }),
        ),
        http.get('/api/control/v1/platform/search/providers', () => {
          listCalls += 1
          if (listCalls === 1) return HttpResponse.json({ data: { providers: [initial] } })
          return listCalls === 2 ? older.promise : latest.promise
        }),
      )
      const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const { user } = renderWithUser(AdminSettingsUnifiedPage, {
        global: { plugins: [[VueQueryPlugin, { queryClient }]] },
      })
      const toggle = await screen.findByRole('switch', { name: 'Enable Unsplash' })
      const section = within(screen.getByRole('region', { name: '图片搜索' }))
      await user.click(toggle)
      await waitFor(() => expect(listCalls).toBe(2))
      await user.type(section.getByLabelText('Unsplash API Key'), 'replacement-key')
      await user.click(section.getByRole('button', { name: '保存' }))
      await waitFor(() => expect(listCalls).toBe(3))
      expect(screen.getByLabelText('Search refresh loading')).toHaveTextContent('true')

      const finishLatest = async () => {
        latest.resolve(
          HttpResponse.json({ data: { providers: [{ ...initial, test_state: 'never' }] } }),
        )
        await list.mock.results[2]!.value
        await nextTick()
      }
      if (order === 'after latest') await finishLatest()
      older.resolve(
        HttpResponse.json(
          { error: { code: 'internal', message: 'older-refresh-failed' } },
          { status: 500 },
        ),
      )
      await expect(list.mock.results[1]!.value).rejects.toThrow()
      await nextTick()
      expect(toast.error).not.toHaveBeenCalled()
      expect(screen.getByLabelText('Search refresh loading')).toHaveTextContent(
        order === 'before latest' ? 'true' : 'false',
      )
      if (order === 'before latest') await finishLatest()
      expect(section.getByText('Unverified')).toBeVisible()
      expect(toggle).toHaveAttribute('data-state', 'unchecked')
      expect(screen.getByLabelText('Search refresh loading')).toHaveTextContent('false')
    },
  )
})
