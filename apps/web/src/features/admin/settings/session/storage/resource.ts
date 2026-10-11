import { requireContractData } from '../../../../../api/contractKernel'
import {
  buildControlMutationHeaders,
  buildRoutineControlMutationHeaders,
} from '../../../shared/index'
import type { ControlMutationOptions } from '../../../shared/index'
import { loadControlSetting } from '../../../shared/index'
import {
  applyControlStorageProfile,
  createControlStorageProfile,
  deleteControlStorageProfile,
  getControlStorageBucketInfo,
  listControlStorageProfiles,
  listControlStorageProviders,
  testControlStorageProfile,
  updateControlStorageProfile,
} from '../../../../../api/generated-query/control/control-platform/control-platform'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from '../../core/storage/profiles'
import { withControlCopy } from '../../../shared/index'

// Admin storage boundary (W2 transports). Reads ride the Control Plane
// with the unified contract envelope (`{ data }` success / `{ error:
// { code, message, request_id } }` errors); save/apply/test/delete move to
// the Control Plane storage operations with the same envelope. Save/apply
// are routine config writes (backend ReasonOptional): no confirm dialog,
// the audit records an empty reason. Delete stays a destructive confirm
// with a required reason. Every mutation carries a header-only
// Idempotency-Key. The deleted `/api/admin/storage/*` routes are gone; this
// module is the single transport.
type GeneratedStorageProviderMeta = {
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

type GeneratedStorageProfile = {
  id: string
  name: string
  provider: string
  enabled: boolean
  is_current: boolean
  builtin: boolean
  secret_configured?: boolean
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

type StorageProfileMutationOptions = ControlMutationOptions

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
  // Routine config write (backend ReasonOptional): the reason may be empty
  // (audit records it as-is) while Idempotency-Key stays required.
  try {
    const body = await (input.id
      ? updateControlStorageProfile(
          input.id,
          { ...(input.payload as unknown as Record<string, unknown>) },
          { headers: buildRoutineControlMutationHeaders(options) },
        )
      : createControlStorageProfile(
          { ...(input.payload as unknown as Record<string, unknown>) },
          { headers: buildRoutineControlMutationHeaders(options) },
        ))
    const saved = requireContractData<{ message: string; profile: GeneratedStorageProfile }>(
      body,
      '保存存储配置失败',
    )
    return {
      message: saved.message,
      profile: normalizeGeneratedProfile(saved.profile),
    }
  } catch (error) {
    throw withControlCopy(error, '保存存储配置失败', {
      control_not_found: '保存存储配置失败',
      not_found: '保存存储配置失败',
      404: '保存存储配置失败',
    })
  }
}

async function applyAdminStorageProfileRecord(
  id: string,
  options: StorageProfileMutationOptions,
): Promise<StorageProfileCommandResult> {
  let body: unknown
  try {
    body = await applyControlStorageProfile(id, {
      headers: buildRoutineControlMutationHeaders(options),
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
  // Connectivity probe is a POST read-guard (backend non-Mutation): no
  // reason ceremony, the audit records an empty reason. Routine headers
  // keep Idempotency-Key while omitting X-Operator-Reason when empty,
  // matching mail routine probes and search provider probes.
  let body: unknown
  try {
    body = await testControlStorageProfile(id, {
      headers: buildRoutineControlMutationHeaders(options),
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
