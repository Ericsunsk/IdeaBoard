import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStorageProfilesResource } from './useStorageProfilesResource'

const storageQueryClientMock = vi.hoisted(() => ({
  invalidateQueries: vi.fn().mockResolvedValue(undefined),
}))

const storageApiMocks = vi.hoisted(() => ({
  applyAdminStorageProfile: vi.fn(),
  createAdminStorageProfile: vi.fn(),
  deleteAdminStorageProfile: vi.fn(),
  getAdminStorageBucketInfo: vi.fn(),
  getListAdminStorageProfilesQueryKey: vi.fn(() => ['admin', 'storage', 'profiles']),
  testAdminStorageProfile: vi.fn(),
  updateAdminStorageProfile: vi.fn(),
  providersRefetch: vi.fn().mockResolvedValue(undefined),
  profilesRefetch: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@tanstack/vue-query', () => ({
  useQueryClient: () => storageQueryClientMock,
  useMutation: (options: {
    mutationFn: (input: unknown) => Promise<unknown>
    onSuccess?: (data: unknown, input: unknown) => Promise<unknown> | unknown
  }) => ({
    mutateAsync: async (input: unknown) => {
      const data = await options.mutationFn(input)
      await options.onSuccess?.(data, input)
      return data
    },
  }),
}))

vi.mock('../../../shared/api/generated-query/admin-storage/admin-storage', () => ({
  applyAdminStorageProfile: storageApiMocks.applyAdminStorageProfile,
  createAdminStorageProfile: storageApiMocks.createAdminStorageProfile,
  deleteAdminStorageProfile: storageApiMocks.deleteAdminStorageProfile,
  getAdminStorageBucketInfo: storageApiMocks.getAdminStorageBucketInfo,
  getListAdminStorageProfilesQueryKey: storageApiMocks.getListAdminStorageProfilesQueryKey,
  testAdminStorageProfile: storageApiMocks.testAdminStorageProfile,
  updateAdminStorageProfile: storageApiMocks.updateAdminStorageProfile,
  useListAdminStorageProviders: () => ({
    data: ref({
      data: {
        providers: [],
      },
    }),
    isLoading: ref(false),
    error: ref(null),
    refetch: storageApiMocks.providersRefetch,
  }),
  useListAdminStorageProfiles: () => ({
    data: ref({
      data: {
        profiles: [],
      },
    }),
    isLoading: ref(false),
    error: ref(null),
    refetch: storageApiMocks.profilesRefetch,
  }),
}))

vi.mock('../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({
    error: vi.fn(),
  }),
}))

describe('useStorageProfilesResource', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    storageQueryClientMock.invalidateQueries.mockResolvedValue(undefined)
    storageApiMocks.getListAdminStorageProfilesQueryKey.mockReturnValue([
      'admin',
      'storage',
      'profiles',
    ])
    storageApiMocks.createAdminStorageProfile.mockResolvedValue({
      code: 0,
      data: {
        message: 'created',
        profile: makeGeneratedProfile('profile-created'),
      },
    })
    storageApiMocks.updateAdminStorageProfile.mockResolvedValue({
      code: 0,
      data: {
        message: 'updated',
        profile: makeGeneratedProfile('profile-updated'),
      },
    })
    storageApiMocks.applyAdminStorageProfile.mockResolvedValue({
      code: 0,
      data: {
        message: 'applied',
        profile: makeGeneratedProfile('profile-applied'),
      },
    })
    storageApiMocks.deleteAdminStorageProfile.mockResolvedValue({
      code: 0,
      data: {
        message: 'deleted',
      },
    })
  })

  it('invalidates the storage profiles query after save, apply, and delete mutations succeed', async () => {
    const resource = useStorageProfilesResource()
    const payload = {
      name: '对象存储',
      provider: 's3-compatible',
      enabled: true,
      config: {
        bucket_name: 'images',
      },
    }

    await resource.saveProfile({ payload })
    await resource.saveProfile({ id: 'profile-1', payload, apply: true })
    await resource.applyProfile('profile-1')
    await resource.deleteProfile('profile-1')

    expect(storageQueryClientMock.invalidateQueries).toHaveBeenCalledTimes(4)
    expect(storageQueryClientMock.invalidateQueries).toHaveBeenNthCalledWith(1, {
      queryKey: ['admin', 'storage', 'profiles'],
    })
    expect(storageQueryClientMock.invalidateQueries).toHaveBeenNthCalledWith(2, {
      queryKey: ['admin', 'storage', 'profiles'],
    })
    expect(storageQueryClientMock.invalidateQueries).toHaveBeenNthCalledWith(3, {
      queryKey: ['admin', 'storage', 'profiles'],
    })
    expect(storageQueryClientMock.invalidateQueries).toHaveBeenNthCalledWith(4, {
      queryKey: ['admin', 'storage', 'profiles'],
    })
    expect(storageApiMocks.updateAdminStorageProfile).toHaveBeenCalledWith(
      'profile-1',
      payload,
      { apply: true },
    )
  })

  it('refetches providers and profiles when loadData runs the existing refresh path', async () => {
    const resource = useStorageProfilesResource()

    await resource.loadData()

    expect(storageApiMocks.providersRefetch).toHaveBeenCalledTimes(1)
    expect(storageApiMocks.profilesRefetch).toHaveBeenCalledTimes(1)
  })
})

function makeGeneratedProfile(id: string) {
  return {
    id,
    name: '对象存储',
    provider: 's3-compatible',
    enabled: true,
    is_current: false,
    builtin: false,
    secret_configured: false,
    config: {
      bucket_name: 'images',
    },
  }
}
