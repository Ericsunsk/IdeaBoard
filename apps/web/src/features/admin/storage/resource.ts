import { api } from '../../../shared/api/client'
import { CONTROL_API_PREFIX } from '../../../shared/api/controlMutator'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'
import {
  getGetControlStorageBucketInfoUrl,
  getListControlStorageProfilesUrl,
  getListControlStorageProvidersUrl,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from './profiles'

// Admin storage boundary (final cut, partial). Reads moved to the Control
// Plane (`GET /api/control/v1/platform/storage/*`, guarded proxies keeping
// the legacy `{code, msg, data}` envelope), so the fetch functions below
// repoint at the generated Control paths with wire types untouched. The
// save/apply/test/delete mutations stay on the legacy routes until the
// admin UI collects the operator reason the Control mutation guard
// requires — see the blocked list.
export type GeneratedStorageProviderMeta = {
  provider: string
  label: string
  description: string
  builtin: boolean
  create_allowed: boolean
  supports_test: boolean
  supports_bucket_info: boolean
  fields?: Array<{
    key: string
    label: string
    type: string
    placeholder?: string
    hint?: string
    required?: boolean
    secret?: boolean
  }>
}

export type GeneratedStorageProfile = {
  id: string
  name: string
  provider: string
  enabled: boolean
  is_current: boolean
  builtin: boolean
  secret_configured?: boolean
  config: Record<string, unknown>
}

export type GeneratedStorageProfilePayload = {
  name: string
  provider: string
  enabled: boolean
  config: Record<string, unknown>
}

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
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ providers: GeneratedStorageProviderMeta[] }>>(
      `${CONTROL_API_PREFIX}${getListControlStorageProvidersUrl()}`,
      { method: 'GET' },
    ),
    '加载存储供应商失败',
  )
  return (data.providers ?? []).map(normalizeGeneratedProvider)
}

async function fetchAdminStorageProfiles(): Promise<StorageProfile[]> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ profiles: GeneratedStorageProfile[] }>>(
      `${CONTROL_API_PREFIX}${getListControlStorageProfilesUrl()}`,
      { method: 'GET' },
    ),
    '加载存储配置失败',
  )
  return (data.profiles ?? []).map(normalizeGeneratedProfile)
}

async function saveAdminStorageProfile(
  input: SaveStorageProfileInput,
): Promise<StorageProfileCommandResult> {
  const query = input.apply ? '?apply=true' : ''
  const url = input.id
    ? `/admin/storage/profiles/${encodeURIComponent(input.id)}${query}`
    : `/admin/storage/profiles${query}`
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ message: string; profile: GeneratedStorageProfile }>>(
      url,
      {
        method: input.id ? 'PUT' : 'POST',
        body: input.payload as unknown as Record<string, unknown>,
      },
    ),
    '保存存储配置失败',
  )
  return {
    message: data.message,
    profile: normalizeGeneratedProfile(data.profile),
  }
}

async function applyAdminStorageProfileRecord(id: string): Promise<StorageProfileCommandResult> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ message: string; profile: GeneratedStorageProfile }>>(
      `/admin/storage/profiles/${encodeURIComponent(id)}/apply`,
      { method: 'POST' },
    ),
    '设为当前失败',
  )
  return {
    message: data.message,
    profile: normalizeGeneratedProfile(data.profile),
  }
}

async function testAdminStorageProfileRecord(id: string): Promise<{ message: string }> {
  return resolveApiData(
    api.request<ApiResponseWithData<{ message: string }>>(
      `/admin/storage/profiles/${encodeURIComponent(id)}/test`,
      { method: 'POST' },
    ),
    '连接失败',
  )
}

async function deleteAdminStorageProfileRecord(id: string): Promise<{ message: string }> {
  return resolveApiData(
    api.request<ApiResponseWithData<{ message: string }>>(
      `/admin/storage/profiles/${encodeURIComponent(id)}`,
      { method: 'DELETE' },
    ),
    '删除失败',
  )
}

async function fetchAdminStorageBucketDomains(id: string): Promise<string[]> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ info: { domains?: string[] } }>>(
      `${CONTROL_API_PREFIX}${getGetControlStorageBucketInfoUrl(id)}`,
      { method: 'GET' },
    ),
    '获取 Bucket 信息失败',
  )
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
