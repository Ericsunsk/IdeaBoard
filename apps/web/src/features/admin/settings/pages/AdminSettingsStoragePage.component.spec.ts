import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen } from '../../../../test/render'
import AdminSettingsStoragePage from './AdminSettingsStoragePage.vue'

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }),
}))

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
    fetchProfiles: vi.fn().mockResolvedValue([
      {
        id: 'storage-1',
        name: '生产环境 OSS',
        provider: 's3-compatible',
        enabled: true,
        is_current: true,
        builtin: false,
        secretConfigured: true,
        config: {
          endpoint: 'oss-cn-shanghai.aliyuncs.com',
          bucket_name: 'prod-bucket',
        },
      },
    ]),
    saveProfile: vi.fn(),
    deleteProfile: vi.fn(),
    testConnection: vi.fn(),
    selectProfile: vi.fn(),
  }),
}))

describe('AdminSettingsStoragePage', () => {
  beforeEach(() => {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/media-upload', () =>
        HttpResponse.json({
          data: { upload_max_concurrent: 4 },
        }),
      ),
    )
  })

  it('renders both storage providers management and upload concurrency table', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(AdminSettingsStoragePage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })

    // 顶部对象存储卡片
    expect(await screen.findByText('生产环境 OSS')).toBeVisible()

    // 底部上传并发设置
    expect(screen.getByRole('heading', { name: '上传并发设置' })).toBeVisible()
    expect(screen.getByRole('table', { name: '上传并发设置列表' })).toBeVisible()
    expect(await screen.findByText('上传并发')).toBeVisible()
  })
})
