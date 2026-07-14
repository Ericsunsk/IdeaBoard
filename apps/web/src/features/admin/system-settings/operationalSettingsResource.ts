import {
  getGetAdminOperationalSettingsQueryKey,
  updateAdminOperationalSettings,
  useGetAdminOperationalSettings,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'
import type {
  OperationalSettingsState,
  UpdateOperationalSettingsRequest,
} from './operationalSettingsContract'

export const operationalSettingsQueryKey = getGetAdminOperationalSettingsQueryKey()
const selectOperationalSettings =
  createApiDataSelector<OperationalSettingsState>('加载运行参数失败')

export function useOperationalSettingsResource() {
  const query = useGetAdminOperationalSettings<OperationalSettingsState>({
    query: { select: selectOperationalSettings },
  })

  return {
    query,
    update: updateOperationalSettings,
  }
}

async function updateOperationalSettings(
  request: UpdateOperationalSettingsRequest,
): Promise<OperationalSettingsState> {
  return resolveApiData(updateAdminOperationalSettings(request), '保存运行参数失败')
}
