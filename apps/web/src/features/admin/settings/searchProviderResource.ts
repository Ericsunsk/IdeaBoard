import { api } from '../../../shared/api/client'
import { CONTROL_API_PREFIX } from '../../../shared/api/controlMutator'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'
import {
  getListControlSearchProvidersUrl,
  testControlSearchProvider,
  updateControlSearchProvider,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
} from '../../../shared/api/contractKernel'
import type {
  SearchProviderId,
  SearchProviderList,
  SearchProviderTestResult,
  SearchProviderUpdate,
} from './searchProviderTypes'

// Admin search provider boundary (final cut). Reads moved to the Control
// Plane (`GET /api/control/v1/platform/search/providers`, a guarded proxy
// keeping the legacy envelope); writes already go through the Control Plane
// (`updateControlSearchProvider` / `testControlSearchProvider`, operator
// reason required). Control marks these routes `x-control-envelope: legacy`,
// so success bodies keep the legacy `{ code, msg, data? }` shape while
// guards (Idempotency-Key + X-Operator-Reason, audit) are enforced.

export type SearchProviderMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

export interface AdminSearchProviderResource {
  list: () => Promise<SearchProviderList>
  update: (
    provider: SearchProviderId,
    input: SearchProviderUpdate,
    options: SearchProviderMutationOptions,
  ) => Promise<void>
  test: (
    provider: SearchProviderId,
    options: SearchProviderMutationOptions,
  ) => Promise<SearchProviderTestResult>
}

export function createAdminSearchProviderResource(): AdminSearchProviderResource {
  return {
    list: listSearchProviders,
    update: updateSearchProvider,
    test: testSearchProvider,
  }
}

function listSearchProviders() {
  return resolveApiData<SearchProviderList>(
    api.request<ApiResponseWithData<SearchProviderList>>(
      `${CONTROL_API_PREFIX}${getListControlSearchProvidersUrl()}`,
      {
        method: 'GET',
      },
    ),
    '加载图片搜索供应商配置失败',
  )
}

async function updateSearchProvider(
  provider: SearchProviderId,
  input: SearchProviderUpdate,
  options: SearchProviderMutationOptions,
): Promise<void> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await updateControlSearchProvider(
      provider,
      { ...input },
      { headers: mutationHeaders(options, operatorReason) },
    )
    assertLegacyOk(body, '保存图片搜索供应商配置失败')
  } catch (error) {
    throw withControlCopy(error, '保存图片搜索供应商配置失败')
  }
}

async function testSearchProvider(
  provider: SearchProviderId,
  options: SearchProviderMutationOptions,
): Promise<SearchProviderTestResult> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await testControlSearchProvider(provider, {
      headers: mutationHeaders(options, operatorReason),
    })
    return readLegacyData<SearchProviderTestResult>(body, '测试图片搜索供应商连接失败')
  } catch (error) {
    throw withControlCopy(error, '测试图片搜索供应商连接失败')
  }
}

function mutationHeaders(options: SearchProviderMutationOptions, operatorReason: string) {
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

function controlError(code: string, message: string): Error & { code?: string | undefined } {
  const error = new Error(message) as Error & { code?: string | undefined }
  error.code = code
  return error
}

// withControlCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text.
function withControlCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError(code === 409 ? 'idempotency_conflict' : code, '操作已提交，请勿重复操作')
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因')
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback)
}

function readErrorCode(error: unknown): string | number | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    if (typeof code === 'string' || typeof code === 'number') return code
  }
  if (error && typeof error === 'object' && 'status' in error) {
    const status = (error as { status?: unknown }).status
    if (status === 409) return 409
  }
  return undefined
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

// Legacy-proxied control bodies keep the `{ code, msg, data? }` shape:
// a numeric `code` of 0 means success, anything else is a failure.
function assertLegacyOk(body: unknown, fallback: string): void {
  if (isRecord(body) && 'code' in body) {
    if (body.code !== 0) {
      const message = typeof body.msg === 'string' && body.msg ? body.msg : fallback
      throw controlError(typeof body.code === 'string' ? body.code : 'control_failed', message)
    }
    return
  }
  if (!isRecord(body)) throw controlError('control_failed', fallback)
}

function readLegacyData<T>(body: unknown, fallback: string): T {
  if (isRecord(body) && 'code' in body) {
    if (body.code !== 0) {
      const message = typeof body.msg === 'string' && body.msg ? body.msg : fallback
      throw controlError(typeof body.code === 'string' ? body.code : 'control_failed', message)
    }
    if (body.data == null) throw controlError('control_failed', fallback)
    return body.data as T
  }
  if (isRecord(body) && 'data' in body && body.data != null) return body.data as T
  throw controlError('control_failed', fallback)
}
