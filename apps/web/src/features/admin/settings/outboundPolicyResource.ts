import { useQuery } from '@tanstack/vue-query'
import {
  getControlOutboundPolicy,
  updateControlOutboundPolicy,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'
import type { OutboundPolicySetting, UpdateOutboundPolicyRequest } from './outboundPolicyContract'

// Admin outbound policy boundary (W2 transports). Reads and writes go
// through the Control Plane (`getControlOutboundPolicy` /
// `updateControlOutboundPolicy`, operator reason required) with the
// contract envelope, so every save carries an explicit operator note and an
// audit record. The deleted `/api/admin/settings/outbound-policy` route is
// gone; this module is the single transport.
export const outboundPolicyQueryKey = ['admin', 'settings', 'outbound-policy'] as const

export type OutboundPolicyMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

export function useOutboundPolicyResource() {
  const query = useQuery<OutboundPolicySetting, Error>({
    queryKey: outboundPolicyQueryKey,
    queryFn: ({ signal }) => fetchOutboundPolicy(signal),
  })

  return {
    query,
    update: updateOutboundPolicy,
  }
}

async function fetchOutboundPolicy(signal?: AbortSignal): Promise<OutboundPolicySetting> {
  try {
    const body = await getControlOutboundPolicy({ signal })
    return requireContractData<OutboundPolicySetting>(body, '加载私有地址访问策略失败')
  } catch (error) {
    throw withControlCopy(error, '加载私有地址访问策略失败')
  }
}

async function updateOutboundPolicy(
  request: UpdateOutboundPolicyRequest,
  options: OutboundPolicyMutationOptions,
): Promise<OutboundPolicySetting> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await updateControlOutboundPolicy(
      { allow_hosts: [...request.allow_hosts], reason: operatorReason },
      {
        headers: buildContractRequestHeaders({
          requestId: newRequestId(),
          idempotencyKey: options.idempotencyKey ?? newRequestId(),
          operatorReason,
        }),
      },
    )
    return requireContractData<OutboundPolicySetting>(body, '保存私有地址访问策略失败')
  } catch (error) {
    throw withControlCopy(error, '保存私有地址访问策略失败')
  }
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
