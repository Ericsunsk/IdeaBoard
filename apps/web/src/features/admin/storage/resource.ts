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
import { resolveApiData } from '../../../shared/api/apiResponse'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from './profiles'

export type SaveStorageProfileInput =
  | {
      id?: undefined
      payload: StorageProfilePayload
      apply?: boolean | undefined
    }
  | {
      id: string
      payload: StorageProfilePayload
      apply?: boolean | undefined
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

async function fetchAdminStorageProviders(): Promise<StorageProviderMeta[]> {
  const data = await resolveApiData(listAdminStorageProviders(), '加载存储供应商失败')
  return (data.providers ?? []).map(normalizeGeneratedProvider)
}

async function fetchAdminStorageProfiles(): Promise<StorageProfile[]> {
  const data = await resolveApiData(listAdminStorageProfiles(), '加载存储配置失败')
  return (data.profiles ?? []).map(normalizeGeneratedProfile)
}

async function saveAdminStorageProfile(
  input: SaveStorageProfileInput,
): Promise<StorageProfileCommandResult> {
  const requestParams = input.apply ? { apply: true } : undefined
  const request = input.id
    ? updateAdminStorageProfile(
        input.id,
        input.payload as GeneratedStorageProfilePayload,
        requestParams,
      )
    : createAdminStorageProfile(input.payload as GeneratedStorageProfilePayload, requestParams)
  const data = await resolveApiData(request, '保存存储配置失败')
  return {
    message: data.message,
    profile: normalizeGeneratedProfile(data.profile),
  }
}

async function applyAdminStorageProfileRecord(id: string): Promise<StorageProfileCommandResult> {
  const data = await resolveApiData(applyAdminStorageProfile(id), '设为当前失败')
  return {
    message: data.message,
    profile: normalizeGeneratedProfile(data.profile),
  }
}

async function testAdminStorageProfileRecord(id: string): Promise<{ message: string }> {
  return resolveApiData(testAdminStorageProfile(id), '连接失败')
}

async function deleteAdminStorageProfileRecord(id: string): Promise<{ message: string }> {
  return resolveApiData(deleteAdminStorageProfile(id), '删除失败')
}

async function fetchAdminStorageBucketDomains(id: string): Promise<string[]> {
  const data = await resolveApiData(getAdminStorageBucketInfo(id), '获取 Bucket 信息失败')
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
