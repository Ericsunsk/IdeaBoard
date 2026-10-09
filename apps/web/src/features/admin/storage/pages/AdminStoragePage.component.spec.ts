import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import { expectDomOrder } from '../../../../test/domOrder'
import { mockAppToast } from '../../../../test/toast'
import AdminStoragePage from './AdminStoragePage.vue'
import AdminStorageTable from '../components/AdminStorageTable.vue'

const storageMocks = vi.hoisted(() => ({
  fetchProfiles: vi.fn(),
  saveProfile: vi.fn(),
  testProfile: vi.fn(),
}))
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
    saveProfile: storageMocks.saveProfile.mockResolvedValue({
      message: 'saved',
      profile: {
        id: 'storage-2',
        name: '备用 MinIO',
        provider: 's3-compatible',
        enabled: true,
        is_current: false,
        builtin: false,
        secret_configured: true,
        config: { endpoint: '127.0.0.1:9000', bucket_name: 'backup-bucket' },
      },
    }),
    testProfile: storageMocks.testProfile.mockResolvedValue({ message: 'tested' }),
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

    const currentSwitch = await screen.findByRole('switch', { name: 'Enable 生产环境 OSS' })
    expect(currentSwitch).toBeDisabled()
    expect(currentSwitch).toHaveAttribute(
      'aria-description',
      '当前存储不能停用，请先切换到其他存储',
    )
    const localSwitch = screen.getByRole('switch', { name: 'Enable 内置本地存储' })
    expect(localSwitch).toBeDisabled()
    expect(localSwitch).toHaveAttribute('aria-description', '内置本地存储始终启用')
    expect(screen.getByRole('switch', { name: 'Enable 备用 MinIO' })).not.toBeDisabled()
  })

  it('routes load failure to toast error without an inline alert or retry button', async () => {
    const mocks = mockAppToast()
    storageMocks.fetchProfiles.mockRejectedValueOnce(new Error('toast-probe-load'))
    const DraftHarness = defineComponent({
      components: { AdminStoragePage },
      data: () => ({ draft: 'existing draft' }),
      template: '<input v-model="draft" aria-label="其他设置草稿" /><AdminStoragePage />',
    })
    const { user } = renderWithUser(DraftHarness)

    // 方案 A：加载失败只 toast，不进 Status；失败一律无内嵌 alert、无重试空态。
    await waitFor(() =>
      expect(mocks.error).toHaveBeenCalledWith(expect.stringContaining('toast-probe-load')),
    )
    expect(mocks.success).not.toHaveBeenCalled()
    expect(screen.queryByText('加载存储配置失败')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重试' })).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByText('Ready')).not.toBeInTheDocument()
    expect(screen.queryByText('Error')).not.toBeInTheDocument()
    expect(screen.queryByText(/toast-probe-load/)).not.toBeInTheDocument()
    // 预设行占位仍在，兄弟草稿不丢失。
    expect(screen.getByText('阿里云 OSS')).toBeInTheDocument()
    const draft = screen.getByRole('textbox', { name: '其他设置草稿' })
    await user.type(draft, ' edited')
    expect(draft).toHaveValue('existing draft edited')
  })

  it('renders storage health pills as the source of truth', async () => {
    renderWithUser(Harness, {
      global: {
        plugins: [],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    // 健康三态（方案 A）：当前→Ready success，载体统一 AdminStatusPill，无英文孤岛。
    const ready = screen.getByText('Ready')
    expect(ready).toBeVisible()
    expect(ready.parentElement?.className ?? '').toContain('bg-success-soft')
    expect(screen.getByText('备用 MinIO')).toBeInTheDocument()
    // standby 已配置不蹭健康色：统一未验证 neutral；预设行同样占位未验证。
    const unverified = screen.getAllByText('Unverified')
    expect(unverified.length).toBeGreaterThan(0)
    expect(unverified[0]?.parentElement?.className ?? '').toContain('bg-muted')
    expect(screen.queryByText('使用中')).not.toBeInTheDocument()
    expect(screen.queryByText('已配置')).not.toBeInTheDocument()
    expect(screen.queryByText('未配置')).not.toBeInTheDocument()
    expect(screen.queryByText('当前生效存储')).not.toBeInTheDocument()
    expect(screen.queryByText('状态')).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Enable' })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Enable 生产环境 OSS' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新增存储' })).toBeInTheDocument()
  })

  it('renders the abnormal pill for enabled-but-incomplete profiles without inline diagnostics', async () => {
    renderWithUser(AdminStorageTable, {
      props: {
        rows: [
          {
            id: 'storage-ok',
            isPresetOnly: false,
            storage: {
              id: 'storage-ok',
              name: '生产环境 OSS',
              provider: 's3-compatible',
              enabled: true,
              isCurrent: true,
              builtin: false,
              secretConfigured: true,
              config: {},
            },
            name: '生产环境 OSS',
            isCurrent: true,
            enabled: true,
            health: 'normal',
            bucket: 'prod-bucket',
            endpoint: 'oss-cn-shanghai.aliyuncs.com',
            supportsTest: false,
            canTest: false,
            canApply: false,
          },
          {
            id: 'storage-bad',
            isPresetOnly: false,
            storage: {
              id: 'storage-bad',
              name: '缺凭据存储',
              provider: 's3-compatible',
              enabled: true,
              isCurrent: false,
              builtin: false,
              secretConfigured: false,
              config: {},
            },
            name: '缺凭据存储',
            isCurrent: false,
            enabled: true,
            health: 'abnormal',
            bucket: '—',
            endpoint: '—',
            supportsTest: false,
            canTest: false,
            canApply: false,
          },
          {
            id: 'preset-aliyun-oss',
            isPresetOnly: true,
            presetKey: 'aliyun-oss',
            name: '阿里云 OSS',
            isCurrent: false,
            enabled: false,
            health: 'unverified',
            bucket: '—',
            endpoint: 'oss-cn-shanghai.aliyuncs.com',
            supportsTest: false,
            canTest: false,
            canApply: false,
          },
        ],
        loading: false,
        applyingId: null,
        testingId: null,
        activeLogoTargetId: null,
      },
    })

    // 方案 A：异常 danger，统一 AdminStatusPill；配置缺失只占位“—”，无内嵌诊断。
    const abnormal = screen.getByText('Error')
    expect(abnormal).toBeVisible()
    expect(abnormal.parentElement?.className ?? '').toContain('bg-destructive-soft')
    expect(screen.getByText('Ready')).toBeVisible()
    expect(screen.getByText('Unverified')).toBeVisible()
    expect(screen.queryByText('使用中')).not.toBeInTheDocument()
    expect(screen.queryByText('已配置')).not.toBeInTheDocument()
    expect(screen.queryByText('未配置')).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('keeps columns in subject, attributes, actions, then enable-switch-last order', async () => {
    renderWithUser(Harness, {
      global: {
        plugins: [],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    const headers = ['Service', 'Status', '存储桶 / 路径', '访问端点 / 域名', '操作', 'Enable'].map(
      (name) => screen.getByRole('columnheader', { name }),
    )
    expectDomOrder(headers)
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
    expect(screen.getByRole('switch', { name: 'Enable 生产环境 OSS' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '新增存储' }))

    const storageDialog = await screen.findByRole('dialog', { name: '新增存储' })
    expect(
      within(storageDialog).queryByRole('switch', { name: '存储启用状态' }),
    ).not.toBeInTheDocument()
  })

  it('keeps the edit dialog free of the redundant current badge', async () => {
    const { user } = renderWithUser(Harness, {
      global: {
        plugins: [],
      },
    })

    expect(await screen.findByText('生产环境 OSS')).toBeInTheDocument()
    // 方案 A：启用真相源是行内开关，弹窗冗余 Status pill 已删除。
    await user.click(screen.getAllByRole('button', { name: '设置' })[0]!)
    const storageDialog = await screen.findByRole('dialog')
    expect(within(storageDialog).queryByText('当前生效存储')).not.toBeInTheDocument()
    expect(within(storageDialog).queryByText('使用中')).not.toBeInTheDocument()
    expect(within(storageDialog).queryByText('已配置')).not.toBeInTheDocument()
  })

  it('flips the storage switch optimistically and keeps the same DOM node on success', async () => {
    let resolveSave!: (value: unknown) => void
    storageMocks.saveProfile.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSave = resolve
      }),
    )
    const { user } = renderWithUser(Harness)
    await screen.findByText('备用 MinIO')
    const toggle = screen.getByRole('switch', { name: 'Enable 备用 MinIO' })
    expect(toggle).not.toBeChecked()

    await user.click(toggle)
    await waitFor(() => expect(storageMocks.saveProfile).toHaveBeenCalledOnce())
    expect(storageMocks.saveProfile).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'storage-2' }),
      { operatorReason: '' },
    )
    // 乐观 intent 同 tick 翻转：同一节点即时 checked + disabled。
    const pending = screen.getByRole('switch', { name: 'Enable 备用 MinIO' })
    expect(pending).toBe(toggle)
    expect(pending).toBeChecked()
    expect(pending).toBeDisabled()

    // 刷新回填与请求一致（否则覆盖 reconcile 后会被旧服务端值盖回）。
    storageMocks.fetchProfiles.mockResolvedValueOnce([
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
        enabled: true,
        is_current: false,
        builtin: false,
        secret_configured: true,
        config: { endpoint: '127.0.0.1:9000', bucket_name: 'backup-bucket' },
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
    ])
    resolveSave({
      message: 'saved',
      profile: {
        id: 'storage-2',
        name: '备用 MinIO',
        provider: 's3-compatible',
        enabled: true,
        is_current: false,
        builtin: false,
        secret_configured: true,
        config: { endpoint: '127.0.0.1:9000', bucket_name: 'backup-bucket' },
      },
    })
    await waitFor(() =>
      expect(screen.getByRole('switch', { name: 'Enable 备用 MinIO' })).toBeEnabled(),
    )
    const toggled = screen.getByRole('switch', { name: 'Enable 备用 MinIO' })
    expect(toggled).toBe(toggle)
    expect(toggled).toBeChecked()
  })

  it('rolls back the storage switch on toggle failure without changing toast semantics', async () => {
    const mocks = mockAppToast()
    let rejectSave!: (error: Error) => void
    storageMocks.saveProfile.mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectSave = reject
      }),
    )
    const { user } = renderWithUser(Harness)
    await screen.findByText('备用 MinIO')
    const toggle = screen.getByRole('switch', { name: 'Enable 备用 MinIO' })

    await user.click(toggle)
    await waitFor(() => expect(storageMocks.saveProfile).toHaveBeenCalledOnce())
    expect(screen.getByRole('switch', { name: 'Enable 备用 MinIO' })).toBeChecked()

    rejectSave(new Error('toggle failed'))
    await waitFor(() =>
      expect(screen.getByRole('switch', { name: 'Enable 备用 MinIO' })).toBeEnabled(),
    )
    const recovered = screen.getByRole('switch', { name: 'Enable 备用 MinIO' })
    expect(recovered).toBe(toggle)
    expect(recovered).not.toBeChecked()
    await waitFor(() => expect(mocks.error).toHaveBeenCalled())
  })
})
