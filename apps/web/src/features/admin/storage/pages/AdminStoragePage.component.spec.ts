import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import AdminStoragePage from './AdminStoragePage.vue'

const storageMocks = vi.hoisted(() => ({ fetchProfiles: vi.fn() }))
vi.mock('../../../../shared/composables/useAppToast')

vi.mock('../../shared/logos/brandLogoResource', () => ({
  useAdminBrandLogoResource: () => ({ save: vi.fn(), remove: vi.fn() }),
}))

vi.mock('../resource', () => ({
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
    fetchProfiles: storageMocks.fetchProfiles.mockResolvedValue([
      {
        id: 'storage-1',
        name: '生产环境 OSS',
        provider: 's3-compatible',
        enabled: true,
        is_current: true,
        builtin: false,
        secret_configured: true,
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
        secret_configured: true,
        config: {
          endpoint: '127.0.0.1:9000',
          bucket_name: 'backup-bucket',
        },
      },
      {
        id: 'storage-local',
        name: '内置本地存储',
        provider: 'local',
        enabled: true,
        is_current: false,
        builtin: true,
        secret_configured: false,
        config: {},
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
    mockAppToast()
  })

  it('disables protected switches with an explanation while allowing ordinary toggles', async () => {
    renderWithUser(Harness)

    const currentSwitch = await screen.findByRole('switch', { name: '启用 生产环境 OSS' })
    expect(currentSwitch).toBeDisabled()
    expect(currentSwitch).toHaveAttribute(
      'aria-description',
      '当前存储不能停用，请先切换到其他存储',
    )
    const localSwitch = screen.getByRole('switch', { name: '启用 内置本地存储' })
    expect(localSwitch).toBeDisabled()
    expect(localSwitch).toHaveAttribute('aria-description', '内置本地存储始终启用')
    expect(screen.getByRole('switch', { name: '启用 备用 MinIO' })).not.toBeDisabled()
  })

  it('recovers from an initial load failure without losing a sibling settings draft', async () => {
    storageMocks.fetchProfiles.mockRejectedValueOnce(new Error('load failed'))
    const DraftHarness = defineComponent({
      components: { AdminStoragePage },
      data: () => ({ draft: 'existing draft' }),
      template: '<input v-model="draft" aria-label="其他设置草稿" /><AdminStoragePage />',
    })
    const { user } = renderWithUser(DraftHarness)

    const retry = await screen.findByRole('button', { name: '重试' })
    expect(screen.getByText('加载存储配置失败')).toBeInTheDocument()
    const draft = screen.getByRole('textbox', { name: '其他设置草稿' })
    await user.type(draft, ' edited')
    await user.click(retry)

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重试' })).not.toBeInTheDocument()
    expect(draft).toHaveValue('existing draft edited')
  })

  it('renders storage cards with active badge and parameters grid', async () => {
    renderWithUser(Harness, {
      global: {
        plugins: [],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    expect(screen.getByText('使用中')).toBeInTheDocument()
    expect(screen.getByText('备用 MinIO')).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: '启用' })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: '启用 生产环境 OSS' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新增存储' })).toBeInTheDocument()
  })

  it('keeps columns in subject, attributes, actions, then enable-switch-last order', async () => {
    renderWithUser(Harness, {
      global: {
        plugins: [],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    const headers = ['Service', 'Status', '存储桶 / 路径', '访问端点 / 域名', '操作', '启用'].map(
      (name) => screen.getByRole('columnheader', { name }),
    )
    for (let index = 0; index < headers.length - 1; index += 1) {
      expect(
        headers[index]!.compareDocumentPosition(headers[index + 1]!) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy()
    }
  })

  it('opens redesigned compact dialog with tabbed pagination for cloud providers', async () => {
    const { user } = renderWithUser(Harness, {
      global: {
        plugins: [],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    // Table rows keep their own apply action; the dialog only has cancel/save.
    expect(screen.getAllByRole('button', { name: '设为当前' }).length).toBeGreaterThan(0)
    const createBtn = screen.getByRole('button', { name: '新增存储' })
    await user.click(createBtn)

    const storageDialog = await screen.findByRole('dialog', { name: '新增存储' })
    expect(
      within(storageDialog).getByText('新增存储', { selector: 'h2:not(.sr-only)' }),
    ).toBeInTheDocument()
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
    expect(
      within(storageDialog).queryByRole('button', { name: '设为当前' }),
    ).not.toBeInTheDocument()
  })

  it('renders rows without search, filter, refresh, or count strip', async () => {
    renderWithUser(Harness, {
      global: {
        plugins: [],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '刷新存储' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('搜索存储对象')).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: '筛选存储状态' })).not.toBeInTheDocument()
    expect(screen.queryByText(/共 \d+ 个存储服务/)).not.toBeInTheDocument()
  })

  it('keeps the create dialog free of the row-level enabled switch', async () => {
    const { user } = renderWithUser(Harness, {
      global: {
        plugins: [],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    // Enabling stays on the table row switch.
    expect(screen.getByRole('switch', { name: '启用 生产环境 OSS' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '新增存储' }))

    const storageDialog = await screen.findByRole('dialog', { name: '新增存储' })
    expect(
      within(storageDialog).queryByRole('switch', { name: '存储启用状态' }),
    ).not.toBeInTheDocument()
  })
})
