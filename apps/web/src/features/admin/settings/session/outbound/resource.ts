import { buildControlMutationHeaders, type ControlMutationOptions } from '../../../shared/index'
import { useQuery } from '@tanstack/vue-query'
import {
  getControlOutboundPolicy,
  updateControlOutboundPolicy,
} from '@/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '@/api/contractKernel'
import { withControlCopy } from '../../../shared/index'
import type {
  OutboundPolicySetting,
  UpdateOutboundPolicyRequest,
} from '../../core/outbound/contract'

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
  options?: OutboundPolicyMutationOptions,
): Promise<OutboundPolicySetting> {
  try {
    const body = await updateControlOutboundPolicy(
      { allow_hosts: [...request.allow_hosts] },
      {
        headers: buildControlMutationHeaders(options),
      },
    )
    return requireContractData<OutboundPolicySetting>(body, '保存私有地址访问策略失败')
  } catch (error) {
    throw withControlCopy(error, '保存私有地址访问策略失败')
  }
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.
