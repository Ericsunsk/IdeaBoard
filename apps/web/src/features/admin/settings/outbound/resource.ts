import {
  buildControlMutationHeaders,
  requireOperatorReason,
  type ControlMutationOptions,
} from '../../shared/lib/controlMutation'
import { useQuery } from '@tanstack/vue-query'
import {
  getControlOutboundPolicy,
  updateControlOutboundPolicy,
} from '../../../../shared/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../../shared/api/contractKernel'
import { withControlCopy } from '../../shared/lib/controlCopy'
import type { OutboundPolicySetting, UpdateOutboundPolicyRequest } from './contract'

// Admin outbound policy boundary (W2 transports). Reads and writes go
// through the Control Plane (`getControlOutboundPolicy` /
// `updateControlOutboundPolicy`, operator reason required) with the
// contract envelope, so every save carries an explicit operator note and an
// audit record. The deleted `/api/admin/settings/outbound-policy` route is
// gone; this module is the single transport.
export const outboundPolicyQueryKey = ['admin', 'settings', 'outbound-policy'] as const

export type OutboundPolicyMutationOptions = ControlMutationOptions

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
        headers: buildControlMutationHeaders(options),
      },
    )
    return requireContractData<OutboundPolicySetting>(body, '保存私有地址访问策略失败')
  } catch (error) {
    throw withControlCopy(error, '保存私有地址访问策略失败')
  }
}

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.
