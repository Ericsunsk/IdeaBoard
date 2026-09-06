import { defineComponent } from 'vue'
import PrimeVue from 'primevue/config'
import ConfirmationService from 'primevue/confirmationservice'
import ToastService from 'primevue/toastservice'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { adminPrimeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen } from '../../../../test/render'
import AdminStoragePage from './AdminStoragePage.vue'

vi.mock('../../shared/logos/adminBrandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))

vi.mock('../../storage/storageResource', () => ({
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
          { key: 'access_key_id', label: 'Access Key ID', type: 'text', required: true },
          {
            key: 'secret_access_key',
            label: 'Secret Access Key',
            type: 'password',
            required: true,
            secret: true,
          },
          { key: 'force_path_style', label: 'Path-style 访问', type: 'switch' },
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
          access_key_id: 'LTAI***',
          secret_access_key: '',
        },
      },
      {
        id: 'storage-2',
        name: '备用 MinIO',
        provider: 's3-compatible',
        enabled: false,
        is_current: false,
        builtin: false,
        secretConfigured: true,
        config: {
          endpoint: '127.0.0.1:9000',
          bucket_name: 'backup-bucket',
        },
      },
    ]),
    applyProfile: vi.fn().mockResolvedValue({ message: 'applied' }),
    deleteProfile: vi.fn().mockResolvedValue({ message: 'deleted' }),
    fetchBucketDomains: vi.fn().mockResolvedValue(['cdn.example.com']),
    saveProfile: vi.fn().mockResolvedValue({ message: 'saved' }),
    testProfile: vi.fn().mockResolvedValue({ message: 'tested' }),
  }),
}))

const Harness = defineComponent({
  components: { AdminStoragePage },
  template: `<AdminStoragePage />`,
})

describe('AdminStoragePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders storage cards with active badge and parameters grid', async () => {
    renderWithUser(Harness, {
      global: {
        plugins: [[PrimeVue, adminPrimeVueTestOptions], ConfirmationService, ToastService],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    expect(screen.getByText('使用中')).toBeInTheDocument()
    expect(screen.getByText('备用 MinIO')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新增存储' })).toBeInTheDocument()
  })

  it('opens redesigned compact dialog with tabbed pagination for cloud providers', async () => {
    const { user } = renderWithUser(Harness, {
      global: {
        plugins: [[PrimeVue, adminPrimeVueTestOptions], ConfirmationService, ToastService],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    const createBtn = screen.getByRole('button', { name: '新增存储' })
    await user.click(createBtn)

    expect(await screen.findByText('添加对象存储')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '阿里云 OSS' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Cloudflare R2' })).toBeInTheDocument()
    expect(screen.getByLabelText(/存储配置名称/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '基础与凭据' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '端点与网络' })).toBeInTheDocument()

    // Switch to Endpoint tab
    const endpointTabBtn = screen.getByRole('button', { name: '端点与网络' })
    await user.click(endpointTabBtn)

    expect(await screen.findByLabelText('Endpoint')).toBeInTheDocument()
    expect(screen.getByLabelText('Bucket 名称')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '保存' })).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: '设为当前' }).length).toBeGreaterThan(0)
  })
})
