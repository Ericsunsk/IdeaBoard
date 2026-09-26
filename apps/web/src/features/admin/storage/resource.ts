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
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from './profiles'

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
      apply?: boolean | undefined
    }
  | {
      id: string
      payload: StorageProfilePayload
      apply?: boolean | undefined
    }

export type StorageProfileMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

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
  testProfile: (
    id: string,
    options: StorageProfileMutationOptions,
  ) => Promise<{ message: string }>
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
  let body: unknown
  try {
    body = await listControlStorageProviders()
  } catch (error) {
    throw withControlCopy(error, '加载存储供应商失败')
  }
  const data = requireContractData<{ providers: GeneratedStorageProviderMeta[] }>(
    body,
    '加载存储供应商失败',
  )
  return (data.providers ?? []).map(normalizeGeneratedProvider)
}

async function fetchAdminStorageProfiles(): Promise<StorageProfile[]> {
  let body: unknown
  try {
    body = await listControlStorageProfiles()
  } catch (error) {
    throw withControlCopy(error, '加载存储配置失败')
  }
  const data = requireContractData<{ profiles: GeneratedStorageProfile[] }>(
    body,
    '加载存储配置失败',
  )
  return (data.profiles ?? []).map(normalizeGeneratedProfile)
}

async function saveAdminStorageProfile(
  input: SaveStorageProfileInput,
  options: StorageProfileMutationOptions,
): Promise<StorageProfileCommandResult> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  const headers = mutationHeaders(options, operatorReason)
  const payload = { ...(input.payload as unknown as Record<string, unknown>) }
  let body: unknown
  try {
    body = input.id
      ? await updateControlStorageProfile(input.id, payload, { headers })
      : await createControlStorageProfile(payload, { headers })
  } catch (error) {
    throw withControlCopy(error, '保存存储配置失败')
  }
  const saved = requireContractData<{ message: string; profile: GeneratedStorageProfile }>(
    body,
    '保存存储配置失败',
  )
    // The legacy `?apply=true` save-and-apply rides the query string, which
    // the generated Control functions do not expose; apply explicitly with
    // the same reason instead of inventing a second command API.
    if (input.apply) {
      return applyAdminStorageProfileRecord(saved.profile.id, {
        operatorReason,
        idempotencyKey: newRequestId(),
      })
    }
    return {
      message: saved.message,
      profile: normalizeGeneratedProfile(saved.profile),
    }
}

async function applyAdminStorageProfileRecord(
  id: string,
  options: StorageProfileMutationOptions,
): Promise<StorageProfileCommandResult> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  let body: unknown
  try {
    body = await applyControlStorageProfile(id, {
      headers: mutationHeaders(options, operatorReason),
    })
  } catch (error) {
    throw withControlCopy(error, '设为当前失败')
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
  const operatorReason = requireOperatorReason(options.operatorReason)
  let body: unknown
  try {
    body = await testControlStorageProfile(id, {
      headers: mutationHeaders(options, operatorReason),
    })
  } catch (error) {
    throw withControlCopy(error, '连接失败')
  }
  return requireContractData<{ message: string }>(body, '连接失败')
}

async function deleteAdminStorageProfileRecord(
  id: string,
  options: StorageProfileMutationOptions,
): Promise<{ message: string }> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  let body: unknown
  try {
    body = await deleteControlStorageProfile(id, {
      headers: mutationHeaders(options, operatorReason),
    })
  } catch (error) {
    throw withControlCopy(error, '删除失败')
  }
  return requireContractData<{ message: string }>(body, '删除失败')
}

async function fetchAdminStorageBucketDomains(id: string): Promise<string[]> {
  let body: unknown
  try {
    body = await getControlStorageBucketInfo(id)
  } catch (error) {
    throw withControlCopy(error, '获取 Bucket 信息失败')
  }
  const data = requireContractData<{ info: { domains?: string[] } }>(body, '获取 Bucket 信息失败')
  return data.info.domains ?? []
}

function mutationHeaders(options: StorageProfileMutationOptions, operatorReason: string) {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? newRequestId(),
    operatorReason,
  })
}

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw controlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function controlError(
  code: string,
  message: string,
  requestId?: string | undefined,
): Error & { code?: string | undefined; requestId?: string | undefined } {
  const error = new Error(message) as Error & {
    code?: string | undefined
    requestId?: string | undefined
  }
  error.code = code
  if (requestId) error.requestId = requestId
  return error
}

// withControlCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text. request_id flows through for
// correlation wherever the app surfaces the error.
function withControlCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  const requestId = readRequestId(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError('idempotency_conflict', '操作已提交，请勿重复操作', requestId)
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因', requestId)
  }
  if (code === 'control_not_found' || code === 'not_found' || code === 404) {
    return controlError('control_not_found', fallback, requestId)
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback, requestId)
}

function readErrorCode(error: unknown): string | number | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    if (typeof code === 'string' || typeof code === 'number') return code
  }
  if (error && typeof error === 'object' && 'status' in error) {
    const status = (error as { status?: unknown }).status
    if (typeof status === 'number') return status
  }
  return undefined
}

function readRequestId(error: unknown): string | undefined {
  if (error && typeof error === 'object') {
    const candidate = error as { requestId?: unknown; request_id?: unknown }
    if (typeof candidate.requestId === 'string' && candidate.requestId) return candidate.requestId
    if (typeof candidate.request_id === 'string' && candidate.request_id) {
      return candidate.request_id
    }
  }
  return undefined
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
