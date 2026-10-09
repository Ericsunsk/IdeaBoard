import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { createDeferred } from '../../../test/deferred'
import { createAdminStorageWorkspace } from './workspace'
import type { AdminStorageResource } from './resource'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from './profiles'

const mockResource = {
  applyProfile: vi.fn(),
  deleteProfile: vi.fn(),
  fetchBucketDomains: vi.fn(),
  fetchProfiles: vi.fn(),
  fetchProviders: vi.fn(),
  saveProfile: vi.fn(),
  testProfile: vi.fn(),
}

beforeEach(() => {
  vi.clearAllMocks()
  mockResource.applyProfile.mockResolvedValue({
    message: 'applied',
    profile: makeProfile({ id: 'profile-applied', is_current: true }),
  })
  mockResource.deleteProfile.mockResolvedValue({ message: 'deleted' })
  mockResource.fetchBucketDomains.mockResolvedValue(['cdn.example.com'])
  mockResource.fetchProfiles.mockResolvedValue([makeProfile()])
  mockResource.fetchProviders.mockResolvedValue([makeS3Provider()])
  mockResource.saveProfile.mockResolvedValue({
    message: 'saved',
    profile: makeProfile({ id: 'profile-saved' }),
  })
  mockResource.testProfile.mockResolvedValue({ message: 'tested' })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('admin storage workspace facade', () => {
  it('surfaces load failures through feedback and recovers on retry', async () => {
    const error = new Error('load failed')
    mockResource.fetchProfiles.mockRejectedValueOnce(error)
    const { workspace, feedback } = createWorkspace()

    await workspace.loadData()
    expect(workspace.storages.value).toEqual([])
    expect(feedback.loadProfilesFailed).toHaveBeenCalledWith(error)

    await workspace.loadData()
    expect(workspace.storages.value).toHaveLength(1)
    expect(workspace.loading.value).toBe(false)
  })

  it.each([false, true])(
    'toggles enabled state to %s without overwriting masked secrets',
    async (enabled) => {
      mockResource.fetchProfiles.mockResolvedValueOnce([makeProfile({ enabled: !enabled })])
      // 刷新回填与请求一致，避免覆盖残留遮挡外部变更的断言失真。
      mockResource.fetchProfiles.mockResolvedValueOnce([makeProfile({ enabled })])
      mockResource.saveProfile.mockResolvedValueOnce({
        message: 'saved',
        profile: makeProfile({ id: 'profile-1', enabled }),
      })
      const { workspace, feedback, confirm } = createWorkspace()
      await workspace.loadData()

      await workspace.toggleEnabled({ id: 'profile-1', name: '对象存储' }, enabled)

      expect(confirm.confirm).not.toHaveBeenCalled()
      expect(mockResource.saveProfile).toHaveBeenCalledWith(
        {
          id: 'profile-1',
          payload: {
            name: '对象存储',
            provider: 's3-compatible',
            enabled,
            config: { bucket_name: 'images' },
          },
        },
        { operatorReason: '' },
      )
      expect(feedback.saveDraftSucceeded).toHaveBeenCalledWith('saved')
      expect(workspace.toggle.isPending('profile-1')).toBe(false)
      // 服务端已追上：覆盖已 reconcile，不再遮挡。
      expect(workspace.toggle.effective('profile-1', enabled)).toBe(enabled)
    },
  )

  it.each([false, true])(
    'keeps saved state %s while refreshing and allows later external changes',
    async (enabled) => {
      const saveRequest = createDeferred<{ message: string; profile: StorageProfile }>()
      const refreshRequest = createDeferred<StorageProfile[]>()
      const savedProfile = makeProfile({ enabled, name: 'Saved storage' })
      mockResource.fetchProfiles.mockResolvedValueOnce([makeProfile({ enabled: !enabled })])
      mockResource.fetchProfiles.mockReturnValueOnce(refreshRequest.promise)
      mockResource.saveProfile.mockReturnValueOnce(saveRequest.promise)
      const { workspace, feedback } = createWorkspace()
      await workspace.loadData()

      const run = workspace.toggleEnabled({ id: 'profile-1', name: '对象存储' }, enabled)
      await vi.waitFor(() => expect(mockResource.saveProfile).toHaveBeenCalledOnce())
      expect(workspace.toggle.isPending('profile-1')).toBe(true)
      expect(workspace.toggle.effective('profile-1', !enabled)).toBe(enabled)
      expect(workspace.storages.value[0]?.enabled).toBe(!enabled)

      saveRequest.resolve({ message: 'saved', profile: savedProfile })
      await vi.waitFor(() => expect(mockResource.fetchProfiles).toHaveBeenCalledTimes(2))

      expect(workspace.storages.value[0]).toMatchObject({
        name: 'Saved storage',
        enabled,
        secretConfigured: true,
        config: { bucket_name: 'images', secret_access_key: '' },
      })
      expect(workspace.toggle.effective('profile-1', workspace.storages.value[0]!.enabled)).toBe(
        enabled,
      )
      expect(workspace.toggle.isPending('profile-1')).toBe(true)

      refreshRequest.resolve([savedProfile])
      await run
      expect(workspace.toggle.isPending('profile-1')).toBe(false)
      expect(feedback.saveDraftSucceeded).toHaveBeenCalledWith('saved')

      mockResource.fetchProfiles.mockResolvedValueOnce([makeProfile({ enabled: !enabled })])
      await workspace.refreshData()
      expect(workspace.toggle.effective('profile-1', workspace.storages.value[0]!.enabled)).toBe(
        !enabled,
      )
    },
  )

  it.each([false, true])(
    'keeps saved state %s and successful save feedback when refresh fails',
    async (enabled) => {
      const refreshError = new Error('refresh failed')
      mockResource.fetchProfiles.mockResolvedValueOnce([makeProfile({ enabled: !enabled })])
      mockResource.fetchProfiles.mockRejectedValueOnce(refreshError)
      mockResource.saveProfile.mockResolvedValueOnce({
        message: 'saved',
        profile: makeProfile({ enabled }),
      })
      const { workspace, feedback } = createWorkspace()
      await workspace.loadData()

      await workspace.toggleEnabled({ id: 'profile-1', name: '对象存储' }, enabled)

      expect(workspace.storages.value[0]?.enabled).toBe(enabled)
      expect(workspace.toggle.effective('profile-1', workspace.storages.value[0]!.enabled)).toBe(
        enabled,
      )
      expect(workspace.storages.value[0]?.config.secret_access_key).toBe('')
      expect(workspace.toggle.isPending('profile-1')).toBe(false)
      expect(feedback.refreshFailed).toHaveBeenCalledWith(refreshError)
      expect(feedback.saveDraftSucceeded).toHaveBeenCalledWith('saved')
      expect(feedback.saveDraftFailed).not.toHaveBeenCalled()
    },
  )

  it('captures toggle failures, leaves the switch state unchanged, and permits retry', async () => {
    const error = new Error('toggle failed')
    const { workspace, feedback } = createWorkspace()
    await workspace.loadData()
    mockResource.saveProfile.mockRejectedValueOnce(error)

    await expect(
      workspace.toggleEnabled({ id: 'profile-1', name: '对象存储' }, false),
    ).resolves.toBeUndefined()

    expect(feedback.saveDraftFailed).toHaveBeenCalledWith(error)
    expect(feedback.saveDraftSucceeded).not.toHaveBeenCalled()
    expect(workspace.storages.value[0]?.enabled).toBe(true)
    expect(workspace.toggle.isPending('profile-1')).toBe(false)
    // 失败回滚：覆盖已删，有效值回到服务端。
    expect(workspace.toggle.effective('profile-1', true)).toBe(true)
    expect(mockResource.fetchProfiles).toHaveBeenCalledTimes(1)

    mockResource.saveProfile.mockResolvedValueOnce({
      message: 'saved',
      profile: makeProfile({ id: 'profile-1', enabled: false }),
    })
    mockResource.fetchProfiles.mockResolvedValueOnce([makeProfile({ enabled: false })])
    await workspace.toggleEnabled({ id: 'profile-1', name: '对象存储' }, false)
    expect(feedback.saveDraftSucceeded).toHaveBeenCalledOnce()
  })

  it.each([{ is_current: true }, { builtin: true, provider: 'local' }])(
    'rejects attempts to disable protected storage %j',
    async (overrides) => {
      mockResource.fetchProfiles.mockResolvedValueOnce([makeProfile(overrides)])
      const { workspace, confirm } = createWorkspace()
      await workspace.loadData()

      await workspace.toggleEnabled({ id: 'profile-1', name: '对象存储' }, false)

      expect(confirm.confirm).not.toHaveBeenCalled()
      expect(mockResource.saveProfile).not.toHaveBeenCalled()
    },
  )

  it('exposes only the table-consumed card projection without search/count scaffolding', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadData()

    expect(Object.keys(workspace.cards).sort()).toEqual(['items'])
    expect(workspace.cards.items.value[0]).toMatchObject({
      providerLabel: 'S3 兼容存储',
      supportsTest: true,
      canTest: true,
      canApply: true,
    })
    expect(workspace.cards.items.value[0]).not.toHaveProperty('statusText')
    expect(workspace.cards.items.value[0]).not.toHaveProperty('statusClass')
    expect(workspace.cards.items.value[0]).not.toHaveProperty('statusSeverity')
    expect(workspace.cards.items.value[0]).not.toHaveProperty('searchableText')
  })

  it('loads providers and profiles into masked card view models', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadData()

    expect(mockResource.fetchProviders).toHaveBeenCalledTimes(1)
    expect(mockResource.fetchProfiles).toHaveBeenCalledTimes(1)
    expect(workspace.storages.value).toMatchObject([
      {
        id: 'profile-1',
        name: '对象存储',
        provider: 's3-compatible',
        secretConfigured: true,
        config: {
          bucket_name: 'images',
          secret_access_key: '',
        },
      },
    ])
    expect(workspace.cards.items.value[0]).toMatchObject({
      providerLabel: 'S3 兼容存储',
      supportsTest: true,
      canTest: true,
      canApply: true,
    })
  })

  it('runs test/apply commands through the workspace and refreshes after apply', async () => {
    const calls: string[] = []
    mockResource.testProfile.mockImplementation(async (id: string) => {
      calls.push(`test:${id}`)
      return { message: 'tested' }
    })
    mockResource.applyProfile.mockImplementation(async (id: string) => {
      calls.push(`apply:${id}`)
      return {
        message: 'applied',
        profile: makeProfile({ id, is_current: true }),
      }
    })
    mockResource.fetchProviders.mockImplementation(async () => {
      calls.push('fetchProviders')
      return [makeS3Provider()]
    })
    mockResource.fetchProfiles.mockImplementation(async () => {
      calls.push('fetchProfiles')
      return [makeProfile()]
    })
    const { workspace, feedback } = createWorkspace()

    await workspace.loadData()
    calls.length = 0

    await workspace.testStorage({ id: 'profile-1', name: '对象存储' })
    await workspace.applyStorage({ id: 'profile-1', name: '对象存储' })

    expect(calls).toEqual(['test:profile-1', 'apply:profile-1', 'fetchProviders', 'fetchProfiles'])
    expect(feedback.testConnectionSucceeded).toHaveBeenCalledWith('tested')
    expect(feedback.applyCurrentSucceeded).toHaveBeenCalledWith('applied')
    expect(workspace.testingId.value).toBeNull()
    expect(workspace.applyingId.value).toBeNull()
  })

  it('keeps test and apply actions in independent pending scopes', async () => {
    const testRequest = createDeferred<{ message: string }>()
    const applyRequest = createDeferred<{
      message: string
      profile: StorageProfile
    }>()
    mockResource.testProfile.mockReturnValueOnce(testRequest.promise)
    mockResource.applyProfile.mockReturnValueOnce(applyRequest.promise)
    const { workspace } = createWorkspace()

    await workspace.loadData()

    const testRun = workspace.testStorage({ id: 'profile-1', name: '对象存储' })
    const applyRun = workspace.applyStorage({ id: 'profile-1', name: '对象存储' })

    // Operator-reason collection yields a microtask before the transport call.
    await vi.waitFor(() => expect(mockResource.testProfile).toHaveBeenCalledOnce())
    await vi.waitFor(() => expect(mockResource.applyProfile).toHaveBeenCalledOnce())
    expect(workspace.testingId.value).toBe('profile-1')
    expect(workspace.applyingId.value).toBe('profile-1')

    testRequest.resolve({ message: 'tested' })
    applyRequest.resolve({
      message: 'applied',
      profile: makeProfile({ is_current: true }),
    })
    await Promise.all([testRun, applyRun])

    expect(workspace.testingId.value).toBeNull()
    expect(workspace.applyingId.value).toBeNull()
  })

  it('refreshes after save commands and maps refresh failures without exposing secrets', async () => {
    const { workspace, feedback } = createWorkspace()
    const refreshError = new Error('refresh failed')

    await workspace.loadData()
    mockResource.fetchProfiles.mockRejectedValueOnce(refreshError)

    await expect(workspace.saveProfile({ payload: makePayload() })).resolves.toMatchObject({
      message: 'saved',
      profile: {
        id: 'profile-saved',
      },
    })

    expect(mockResource.saveProfile).toHaveBeenCalledWith(
      { payload: makePayload() },
      { operatorReason: '' },
    )
    expect(feedback.refreshFailed).toHaveBeenCalledWith(refreshError)
    expect(workspace.storages.value[0]?.config.secret_access_key).toBe('')
  })

  it('maps stale targets and command errors at the workspace boundary', async () => {
    const error = new Error('apply failed')
    const { workspace, feedback } = createWorkspace()

    await workspace.applyStorage({ id: 'missing', name: 'Missing' })

    expect(feedback.staleProfile).toHaveBeenCalledTimes(1)
    expect(mockResource.applyProfile).not.toHaveBeenCalled()

    await workspace.loadData()
    mockResource.applyProfile.mockRejectedValueOnce(error)

    await workspace.applyStorage({ id: 'profile-1', name: '对象存储' })

    expect(feedback.applyCurrentFailed).toHaveBeenCalledWith(error)
    expect(feedback.applyCurrentSucceeded).not.toHaveBeenCalled()
    expect(mockResource.fetchProfiles).toHaveBeenCalledTimes(1)
  })

  it('runs storage connection probes without a confirm dialog', async () => {
    const { workspace, confirm } = createWorkspace()

    await workspace.loadData()
    await workspace.testStorage({ id: 'profile-1', name: '对象存储' })

    expect(confirm.confirm).not.toHaveBeenCalled()
    // 探针 reason 对齐 routine 空 reason（后端 POST read-guard 非 Mutation，
    // 无需 reason/Idempotency 仪式，审计记空 reason）。
    expect(mockResource.testProfile).toHaveBeenCalledWith('profile-1', {
      operatorReason: '',
    })
  })

  it('keeps delete confirmation and refresh sequencing inside the workspace', async () => {
    const { workspace, feedback, confirm } = createWorkspace()

    await workspace.loadData()
    await workspace.requestRemoveProfile({ id: 'profile-1', name: '对象存储' })

    expect(confirm.confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认删除存储',
        message: '确定删除存储「对象存储」吗？此操作不可恢复，且将记录操作人与原因。',
        confirmLabel: '确定删除',
        intent: 'danger',
        reason: { required: true },
      }),
    )

    expect(mockResource.deleteProfile).toHaveBeenCalledWith('profile-1', {
      operatorReason: 'storage review',
    })
    expect(feedback.profileDeleted).toHaveBeenCalledWith('deleted')
    expect(mockResource.fetchProfiles).toHaveBeenCalledTimes(2)
  })
})

function createWorkspace() {
  const feedback = createStorageFeedback()
  const confirm = {
    confirm: vi.fn().mockResolvedValue(true),
    readReason: vi.fn().mockReturnValue('storage review'),
  }
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminStorageWorkspace({
    feedback,
    confirm,
    runAsyncAction,
    resource: mockResource as unknown as AdminStorageResource,
  })
  return { workspace, feedback, confirm }
}

function createStorageFeedback() {
  return {
    loadProfilesFailed: vi.fn(),
    refreshFailed: vi.fn(),
    staleProfile: vi.fn(),
    unsupportedVendorPreset: vi.fn(),
    noCreatableProviders: vi.fn(),
    missingEditableTarget: vi.fn(),
    saveDraftSucceeded: vi.fn(),
    saveDraftFailed: vi.fn(),
    applyCurrentSucceeded: vi.fn(),
    applyCurrentFailed: vi.fn(),
    testConnectionSucceeded: vi.fn(),
    testConnectionFailed: vi.fn(),
    bucketDomainsDetected: vi.fn(),
    bucketDomainsEmpty: vi.fn(),
    fetchBucketDomainsFailed: vi.fn(),
    profileDeleted: vi.fn(),
    deleteProfileFailed: vi.fn(),
  }
}

function makeS3Provider(): StorageProviderMeta {
  return {
    provider: 's3-compatible',
    label: 'S3 兼容存储',
    description: 'S3 compatible object storage',
    builtin: false,
    create_allowed: true,
    supports_test: true,
    supports_bucket_info: true,
    fields: [
      {
        key: 'bucket_name',
        label: 'Bucket',
        type: 'text',
        required: true,
      },
      {
        key: 'secret_access_key',
        label: 'Access Key Secret',
        type: 'password',
        required: true,
        secret: true,
      },
    ],
  }
}

function makeProfile(overrides: Partial<StorageProfile> = {}): StorageProfile {
  return {
    id: 'profile-1',
    name: '对象存储',
    provider: 's3-compatible',
    enabled: true,
    is_current: false,
    builtin: false,
    secret_configured: true,
    config: {
      bucket_name: 'images',
      secret_access_key: 'server-secret',
    },
    ...overrides,
  }
}

function makePayload(): StorageProfilePayload {
  return {
    name: '对象存储',
    provider: 's3-compatible',
    enabled: true,
    config: {
      bucket_name: 'images',
      secret_access_key: 'new-secret',
    },
  }
}
