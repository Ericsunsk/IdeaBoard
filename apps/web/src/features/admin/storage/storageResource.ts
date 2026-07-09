import {
  applyAdminStorageProfile,
  createAdminStorageProfile,
  deleteAdminStorageProfile,
  getAdminStorageBucketInfo,
  listAdminStorageProfiles,
  listAdminStorageProviders,
  testAdminStorageProfile,
  updateAdminStorageProfile,
} from '../../../shared/api/generated-query/admin-storage/admin-storage'
import type {
  StorageProfile as GeneratedStorageProfile,
  StorageProfilePayload as GeneratedStorageProfilePayload,
  StorageProviderMeta as GeneratedStorageProviderMeta,
} from '../../../shared/api/generated-query/model'
import { requireApiData } from '../shared/queries/adminApiResponse'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from './storageHelpers'

export type SaveStorageProfileInput =
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

export type StorageProfileCommandResult = {
  profile: StorageProfile
  message: string
}

export type AdminStorageResource = {
  fetchProviders: () => Promise<StorageProviderMeta[]>
  fetchProfiles: () => Promise<StorageProfile[]>
  saveProfile: (input: SaveStorageProfileInput) => Promise<StorageProfileCommandResult>
  applyProfile: (id: string) => Promise<StorageProfileCommandResult>
  testProfile: (id: string) => Promise<{ message: string }>
  deleteProfile: (id: string) => Promise<{ message: string }>
  fetchBucketDomains: (id: string) => Promise<string[]>
}

export function createAdminStorageResource(): AdminStorageResource {
  return {
    fetchProviders: fetchAdminStorageProviders,
    fetchProfiles: fetchAdminStorageProfiles,
    saveProfile: saveAdminStorageProfile,
    applyProfile: applyAdminStorageProfileRecord,
    testProfile: testAdminStorageProfileRecord,
    deleteProfile: deleteAdminStorageProfileRecord,
    fetchBucketDomains: fetchAdminStorageBucketDomains,
  }
}

export async function fetchAdminStorageProviders(): Promise<StorageProviderMeta[]> {
  const data = requireApiData(await listAdminStorageProviders(), '加载存储供应商失败')
  return (data.providers ?? []).map(normalizeGeneratedProvider)
}

export async function fetchAdminStorageProfiles(): Promise<StorageProfile[]> {
  const data = requireApiData(await listAdminStorageProfiles(), '加载存储配置失败')
  return (data.profiles ?? []).map(normalizeGeneratedProfile)
}

export async function saveAdminStorageProfile(
  input: SaveStorageProfileInput,
): Promise<StorageProfileCommandResult> {
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
}

export async function applyAdminStorageProfileRecord(
  id: string,
): Promise<StorageProfileCommandResult> {
  const data = requireApiData(await applyAdminStorageProfile(id), '设为当前失败')
  return {
    message: data.message,
    profile: normalizeGeneratedProfile(data.profile),
  }
}

export async function testAdminStorageProfileRecord(id: string): Promise<{ message: string }> {
  return requireApiData(await testAdminStorageProfile(id), '连接失败')
}

export async function deleteAdminStorageProfileRecord(id: string): Promise<{ message: string }> {
  return requireApiData(await deleteAdminStorageProfile(id), '删除失败')
}

export async function fetchAdminStorageBucketDomains(id: string): Promise<string[]> {
  const data = requireApiData(await getAdminStorageBucketInfo(id), '获取 Bucket 信息失败')
  return data.info.domains ?? []
}

function normalizeGeneratedProvider(provider: GeneratedStorageProviderMeta): StorageProviderMeta {
  return {
    provider: provider.provider,
    label: provider.label,
    description: provider.description,
    builtin: provider.builtin,
    create_allowed: provider.create_allowed,
    supports_test: provider.supports_test,
    supports_bucket_info: provider.supports_bucket_info,
    fields: (provider.fields ?? []).map((field) => ({ ...field })),
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
