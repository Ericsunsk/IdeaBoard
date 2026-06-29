import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, watch } from 'vue'
import {
  applyAdminStorageProfile,
  createAdminStorageProfile,
  deleteAdminStorageProfile,
  getAdminStorageBucketInfo,
  getListAdminStorageProfilesQueryKey,
  testAdminStorageProfile,
  updateAdminStorageProfile,
  useListAdminStorageProfiles,
  useListAdminStorageProviders,
} from '../../../shared/api/generated-query/admin-storage/admin-storage'
import type {
  ApiResponseBase,
  StorageProfile as GeneratedStorageProfile,
  StorageProfilePayload as GeneratedStorageProfilePayload,
} from '../../../shared/api/generated-query/model'
import { useAppToast } from '../../../shared/composables/useAppToast'
import {
  formatDisplayErrorMessage,
  sanitizeApiErrorMessage,
} from '../../../shared/utils/errorMessages'
import {
  normalizeProfile,
  toProviderMetaMap,
  type StorageCard,
  type StorageProfile,
  type StorageProfilePayload,
  type StorageProviderMeta,
} from './storageHelpers'

type SaveStorageProfileInput =
  | {
      id?: undefined
      payload: StorageProfilePayload
      apply?: boolean
    }
  | {
      id: string
      payload: StorageProfilePayload
      apply?: boolean
    }

export function useStorageProfilesResource() {
  const appToast = useAppToast()
  const queryClient = useQueryClient()
  const providersQuery = useListAdminStorageProviders()
  const profilesQuery = useListAdminStorageProfiles()

  const providers = computed<StorageProviderMeta[]>(
    () => providersQuery.data.value?.data?.providers ?? [],
  )
  const providerMap = computed(() => toProviderMetaMap(providers.value))
  const storages = computed<StorageCard[]>(() =>
    (profilesQuery.data.value?.data?.profiles ?? []).map((profile) =>
      normalizeProfile(normalizeGeneratedProfile(profile), providerMap.value),
    ),
  )
  const loading = computed(() => providersQuery.isLoading.value || profilesQuery.isLoading.value)
  const creatableProviders = computed(() =>
    providers.value.filter((provider) => provider.create_allowed),
  )

  const saveProfileMutation = useMutation({
    mutationFn: async (input: SaveStorageProfileInput) => {
      const requestParams = input.apply ? { apply: true } : undefined
      const response = input.id
        ? await updateAdminStorageProfile(
            input.id,
            input.payload as GeneratedStorageProfilePayload,
            requestParams,
          )
        : await createAdminStorageProfile(
            input.payload as GeneratedStorageProfilePayload,
            requestParams,
          )
      const data = requireApiData(response, '保存存储配置失败')
      return {
        message: data.message,
        profile: normalizeGeneratedProfile(data.profile),
      }
    },
    onSuccess: invalidateProfiles,
  })
  const applyProfileMutation = useMutation({
    mutationFn: async (id: string) => {
      const data = requireApiData(await applyAdminStorageProfile(id), '设为当前失败')
      return {
        message: data.message,
        profile: normalizeGeneratedProfile(data.profile),
      }
    },
    onSuccess: invalidateProfiles,
  })
  const testProfileMutation = useMutation({
    mutationFn: async (id: string) => requireApiData(await testAdminStorageProfile(id), '连接失败'),
  })
  const deleteProfileMutation = useMutation({
    mutationFn: async (id: string) =>
      requireApiData(await deleteAdminStorageProfile(id), '删除失败'),
    onSuccess: invalidateProfiles,
  })
  const bucketDomainsMutation = useMutation({
    mutationFn: async (id: string) => {
      const data = requireApiData(await getAdminStorageBucketInfo(id), '获取 Bucket 信息失败')
      return data.info.domains ?? []
    },
  })

  watch(
    () => providersQuery.error.value || profilesQuery.error.value,
    (error) => {
      if (error) appToast.error(formatDisplayErrorMessage(error, '加载存储配置失败'))
    },
  )

  function invalidateProfiles() {
    return queryClient.invalidateQueries({ queryKey: getListAdminStorageProfilesQueryKey() })
  }

  async function loadData() {
    await Promise.all([providersQuery.refetch(), profilesQuery.refetch()])
  }

  return {
    loading,
    providers,
    providerMap,
    creatableProviders,
    storages,
    saveProfile: saveProfileMutation.mutateAsync,
    applyProfile: applyProfileMutation.mutateAsync,
    testProfile: testProfileMutation.mutateAsync,
    deleteProfile: deleteProfileMutation.mutateAsync,
    fetchBucketDomains: bucketDomainsMutation.mutateAsync,
    loadData,
  }
}

function normalizeGeneratedProfile(profile: GeneratedStorageProfile): StorageProfile {
  return {
    ...profile,
    provider: profile.provider,
    secret_configured: profile.secret_configured ?? false,
    config: profile.config as Record<string, unknown>,
  }
}

function requireApiData<T>(response: ApiResponseBase & { data?: T }, fallback: string): T {
  if (response.code !== 0 || response.data == null) {
    throw new Error(sanitizeApiErrorMessage(response.msg, fallback))
  }
  return response.data
}
