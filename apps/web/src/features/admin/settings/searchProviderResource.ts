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
import { withControlCopy } from '../shared/lib/controlCopy'
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

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.
