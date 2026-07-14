import { useMutation, useQueryClient } from '@tanstack/vue-query'
import {
  getGetAdminOperationalSettingsQueryKey,
  updateAdminOperationalSettings,
  useGetAdminOperationalSettings,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { ApiResponseOperationalSettingsState } from '../../../shared/api/generated-query/model'
import { requireApiData } from '../../../shared/api/apiResponse'
import type {
  OperationalSettingsState,
  UpdateOperationalSettingsRequest,
} from './operationalSettingsContract'

export const operationalSettingsQueryKey = getGetAdminOperationalSettingsQueryKey()

export function useOperationalSettingsResource() {
  const queryClient = useQueryClient()
  const query = useGetAdminOperationalSettings()
  const updateMutation = useMutation({
    mutationFn: updateOperationalSettings,
    onSuccess: (state) => {
      queryClient.setQueryData<ApiResponseOperationalSettingsState>(operationalSettingsQueryKey, {
        code: 0,
        msg: 'ok',
        data: state,
      })
    },
  })

  return {
    query,
    update: updateMutation.mutateAsync,
    updatePending: updateMutation.isPending,
  }
}

async function updateOperationalSettings(
  request: UpdateOperationalSettingsRequest,
): Promise<OperationalSettingsState> {
  const response = await updateAdminOperationalSettings(request)
  return requireApiData(response, '保存运行参数失败')
}
