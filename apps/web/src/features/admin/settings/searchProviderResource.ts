import {
  listControlSearchProviders,
  testControlSearchProvider,
  updateControlSearchProvider,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'
import type {
  SearchProviderId,
  SearchProviderList,
  SearchProviderTestResult,
  SearchProviderUpdate,
} from './searchProviderTypes'

// Admin search provider boundary (final cut). Reads ride the generated
// Control hook (`listControlSearchProviders`); writes already go through
// the Control Plane (`updateControlSearchProvider` /
// `testControlSearchProvider`, operator reason required). All routes use
// the unified contract envelope (`{ data }` success / `{ error: { code,
// message, request_id } }` errors); only the binary brand-logo and SSE
// monitor stream stay raw. Guards (Idempotency-Key + X-Operator-Reason,
// audit) are enforced.

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
  // Wire-identical to the retired hand-rolled GET (same `{ data }` body
  // through `controlMutator`); the generated hook owns the route so the
  // duplicate URL prefixing is deleted.
  return (async () => {
    try {
      const body = await listControlSearchProviders()
      return requireContractData<SearchProviderList>(body, '加载图片搜索供应商配置失败')
    } catch (error) {
      throw withControlCopy(error, '加载图片搜索供应商配置失败')
    }
  })()
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
    // Nil-ack mutation: success is `{ data: {} }`.
    requireContractData<Record<string, unknown>>(body, '保存图片搜索供应商配置失败')
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
    return requireContractData<SearchProviderTestResult>(body, '测试图片搜索供应商连接失败')
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
