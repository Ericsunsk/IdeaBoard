import {
  buildControlMutationHeaders,
  type ControlMutationOptions,
} from '../shared/lib/controlMutation'
import { loadControlSetting, storeControlSetting } from '../shared/lib/controlResource'
import {
  applyControlStorageProfile,
  createControlStorageProfile,
  deleteControlStorageProfile,
  getControlStorageBucketInfo,
  listControlStorageProfiles,
  listControlStorageProviders,
  testControlStorageProfile,
  updateControlStorageProfile,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../shared/api/contractKernel'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from './profiles'
import { withControlCopy } from '../shared/lib/controlCopy'

// Admin storage boundary (W2 transports). Reads ride the Control Plane
// with the unified contract envelope (`{ data }` success / `{ error:
// { code, message, request_id } }` errors); save/apply/test/delete move to
// the Control Plane storage operations with the same envelope. Every
// mutation carries a header-only Idempotency-Key plus an operator reason
// collected through the shared confirm dialog, and emits a control audit
// record. The deleted `/api/admin/storage/*` routes are gone; this module
// is the single transport.
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
    }
  | {
      id: string
      payload: StorageProfilePayload
    }

export type StorageProfileMutationOptions = ControlMutationOptions

export type StorageProfileCommandResult = {
  profile: StorageProfile
  message: string
}

export type AdminStorageResource = {
  fetchProviders: () => Promise<StorageProviderMeta[]>
  fetchProfiles: () => Promise<StorageProfile[]>
  saveProfile: (
    input: SaveStorageProfileInput,
    options: StorageProfileMutationOptions,
  ) => Promise<StorageProfileCommandResult>
  applyProfile: (
    id: string,
    options: StorageProfileMutationOptions,
  ) => Promise<StorageProfileCommandResult>
  testProfile: (id: string, options: StorageProfileMutationOptions) => Promise<{ message: string }>
  deleteProfile: (
    id: string,
    options: StorageProfileMutationOptions,
  ) => Promise<{ message: string }>
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
  return loadControlSetting(listControlStorageProviders, {
    fallback: '加载存储供应商失败',
    domainMessages: {
      control_not_found: '加载存储供应商失败',
      not_found: '加载存储供应商失败',
      404: '加载存储供应商失败',
    },
    map: (data: { providers: GeneratedStorageProviderMeta[] }) =>
      (data.providers ?? []).map(normalizeGeneratedProvider),
  })
}

async function fetchAdminStorageProfiles(): Promise<StorageProfile[]> {
  return loadControlSetting(listControlStorageProfiles, {
    fallback: '加载存储配置失败',
    domainMessages: {
      control_not_found: '加载存储配置失败',
      not_found: '加载存储配置失败',
      404: '加载存储配置失败',
    },
    map: (data: { profiles: GeneratedStorageProfile[] }) =>
      (data.profiles ?? []).map(normalizeGeneratedProfile),
  })
}

async function saveAdminStorageProfile(
  input: SaveStorageProfileInput,
  options: StorageProfileMutationOptions,
): Promise<StorageProfileCommandResult> {
  return storeControlSetting(input, options, {
    fallback: '保存存储配置失败',
    domainMessages: {
      control_not_found: '保存存储配置失败',
      not_found: '保存存储配置失败',
      404: '保存存储配置失败',
    },
    toRequest: (save: SaveStorageProfileInput) => ({
      ...(save.payload as unknown as Record<string, unknown>),
    }),
    persist: (body, headers) =>
      input.id
        ? updateControlStorageProfile(input.id, body as Record<string, unknown>, { headers })
        : createControlStorageProfile(body as Record<string, unknown>, { headers }),
    map: (saved: { message: string; profile: GeneratedStorageProfile }) => ({
      message: saved.message,
      profile: normalizeGeneratedProfile(saved.profile),
    }),
  })
}

async function applyAdminStorageProfileRecord(
  id: string,
  options: StorageProfileMutationOptions,
): Promise<StorageProfileCommandResult> {
  let body: unknown
  try {
    body = await applyControlStorageProfile(id, {
      headers: buildControlMutationHeaders(options),
    })
  } catch (error) {
    throw withControlCopy(error, '设为当前失败', {
      control_not_found: '设为当前失败',
      not_found: '设为当前失败',
      404: '设为当前失败',
    })
  }
  const applied = requireContractData<{ message: string; profile: GeneratedStorageProfile }>(
    body,
    '设为当前失败',
  )
  return {
    message: applied.message,
    profile: normalizeGeneratedProfile(applied.profile),
  }
}

async function testAdminStorageProfileRecord(
  id: string,
  options: StorageProfileMutationOptions,
): Promise<{ message: string }> {
  let body: unknown
  try {
    body = await testControlStorageProfile(id, {
      headers: buildControlMutationHeaders(options),
    })
  } catch (error) {
    throw withControlCopy(error, '连接失败', {
      control_not_found: '连接失败',
      not_found: '连接失败',
      404: '连接失败',
    })
  }
  return requireContractData<{ message: string }>(body, '连接失败')
}

async function deleteAdminStorageProfileRecord(
  id: string,
  options: StorageProfileMutationOptions,
): Promise<{ message: string }> {
  let body: unknown
  try {
    body = await deleteControlStorageProfile(id, {
      headers: buildControlMutationHeaders(options),
    })
  } catch (error) {
    throw withControlCopy(error, '删除失败', {
      control_not_found: '删除失败',
      not_found: '删除失败',
      404: '删除失败',
    })
  }
  return requireContractData<{ message: string }>(body, '删除失败')
}

async function fetchAdminStorageBucketDomains(id: string): Promise<string[]> {
  let body: unknown
  try {
    body = await getControlStorageBucketInfo(id)
  } catch (error) {
    throw withControlCopy(error, '获取 Bucket 信息失败', {
      control_not_found: '获取 Bucket 信息失败',
      not_found: '获取 Bucket 信息失败',
      404: '获取 Bucket 信息失败',
    })
  }
  const data = requireContractData<{ info: { domains?: string[] } }>(body, '获取 Bucket 信息失败')
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
