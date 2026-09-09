import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { createDeferred } from '../../../test/deferred'
import { createAdminStorageWorkspace } from './workspace'
import type { AdminStorageResource } from './resource'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from './helpers'

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
      statusText: '已配置',
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

    expect(mockResource.testProfile).toHaveBeenCalledOnce()
    expect(mockResource.applyProfile).toHaveBeenCalledOnce()
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

    expect(mockResource.saveProfile).toHaveBeenCalledWith({ payload: makePayload() })
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

  it('keeps delete confirmation and refresh sequencing inside the workspace', async () => {
    const { workspace, feedback, confirm } = createWorkspace()

    await workspace.loadData()
    workspace.requestRemoveProfile({ id: 'profile-1', name: '对象存储' })

    expect(confirm.request).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认删除存储',
        message: '确定删除存储「对象存储」吗？此操作不可恢复。',
        confirmLabel: '确定删除',
        intent: 'danger',
      }),
    )

    await confirm.request.mock.calls[0]?.[0].onConfirm()

    expect(mockResource.deleteProfile).toHaveBeenCalledWith('profile-1')
    expect(feedback.profileDeleted).toHaveBeenCalledWith('deleted')
    expect(mockResource.fetchProfiles).toHaveBeenCalledTimes(2)
  })
})

function createWorkspace() {
  const feedback = createStorageFeedback()
  const confirm = { request: vi.fn() }
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
    saveAndApplySucceeded: vi.fn(),
    saveAndApplyFailed: vi.fn(),
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
