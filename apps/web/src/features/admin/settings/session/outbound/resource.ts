import {
  buildRoutineControlMutationHeaders,
  type ControlMutationOptions,
} from '../../../shared/index'
import { useQuery } from '@tanstack/vue-query'
import {
  getControlOutboundPolicy,
  updateControlOutboundPolicy,
} from '../../../../../api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../../../api/contractKernel'
import { withControlCopy } from '../../../shared/index'
import type {
  OutboundPolicySetting,
  UpdateOutboundPolicyRequest,
} from '../../core/outbound/contract'

// Admin outbound policy boundary (W2 transports). Reads and writes go
// through the Control Plane (`getControlOutboundPolicy` /
// `updateControlOutboundPolicy`) with the contract envelope; the update is a
// routine config write (backend ReasonOptional), so saves skip the confirm
// dialog and audit an empty reason while keeping the header-only
// Idempotency-Key. The deleted `/api/admin/settings/outbound-policy` route
// is gone; this module is the single transport.
export const outboundPolicyQueryKey = ['admin', 'settings', 'outbound-policy'] as const

type OutboundPolicyMutationOptions = ControlMutationOptions

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
  // Routine config write (backend ReasonOptional): the reason may be empty
  // (audit records it as-is) while Idempotency-Key stays required.
  const operatorReason = options.operatorReason.trim()
  try {
    const body = await updateControlOutboundPolicy(
      { allow_hosts: [...request.allow_hosts], reason: operatorReason },
      {
        headers: buildRoutineControlMutationHeaders(options),
      },
    )
    return requireContractData<OutboundPolicySetting>(body, '保存私有地址访问策略失败')
  } catch (error) {
    throw withControlCopy(error, '保存私有地址访问策略失败')
  }
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.
