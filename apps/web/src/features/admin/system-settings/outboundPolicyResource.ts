import {
  getGetAdminOutboundPolicyQueryKey,
  updateAdminOutboundPolicy,
  useGetAdminOutboundPolicy,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'
import type {
  OutboundPolicySetting,
  UpdateOutboundPolicyRequest,
} from './outboundPolicyContract'

export const outboundPolicyQueryKey = getGetAdminOutboundPolicyQueryKey()
const selectOutboundPolicy =
  createApiDataSelector<OutboundPolicySetting>('加载私有地址访问策略失败')

export function useOutboundPolicyResource() {
  const query = useGetAdminOutboundPolicy<OutboundPolicySetting>({
    query: { select: selectOutboundPolicy },
  })

  return {
    query,
    update: updateOutboundPolicy,
  }
}

async function updateOutboundPolicy(
  request: UpdateOutboundPolicyRequest,
): Promise<OutboundPolicySetting> {
  return resolveApiData(updateAdminOutboundPolicy(request), '保存私有地址访问策略失败')
}
