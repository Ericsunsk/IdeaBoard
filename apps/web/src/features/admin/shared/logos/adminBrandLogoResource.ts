import {
  deleteAdminBrandLogoOverride,
  getListAdminBrandLogoOverridesQueryKey,
  setAdminBrandLogoOverride,
} from '../../../../shared/api/generated-query/admin-brand-logos/admin-brand-logos'
import type {
  AdminBrandLogoOverride as AdminBrandLogoOverrideDto,
  ApiResponseAdminBrandLogoOverrideList,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData, resolveApiOk } from '../../../../shared/api/apiResponse'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import {
  replaceAdminBrandLogoOverrides,
  type AdminBrandLogoKey,
  type AdminBrandLogoScope,
} from './adminBrandLogos'

type BrandLogoOverrideQueryClient = Pick<
  QueryClient,
  'cancelQueries' | 'getQueryData' | 'setQueryData'
>

export function hydrateAdminBrandLogoOverrides(
  overrides: Array<{ scope: AdminBrandLogoScope; target_key: string; logo_key: string }>,
) {
  replaceAdminBrandLogoOverrides(
    overrides.map((override) => ({
      scope: override.scope,
      targetKey: override.target_key,
      logoKey: override.logo_key as AdminBrandLogoKey,
    })),
  )
}

export function createAdminBrandLogoResource(queryClient: BrandLogoOverrideQueryClient) {
  return {
    async save(scope: AdminBrandLogoScope, targetKey: string | number, logoKey: AdminBrandLogoKey) {
      await cancelBrandLogoOverrideQuery(queryClient)
      const data = await resolveApiData(
        setAdminBrandLogoOverride(scope, String(targetKey), { logo_key: logoKey }),
        '保存品牌 Logo 配置失败',
      )
      syncBrandLogoOverrideQuery(queryClient, data, false)
    },

    async remove(scope: AdminBrandLogoScope, targetKey: string | number) {
      const normalizedTargetKey = String(targetKey)
      await cancelBrandLogoOverrideQuery(queryClient)
      await resolveApiOk(
        deleteAdminBrandLogoOverride(scope, normalizedTargetKey),
        '恢复智能识别失败',
      )
      syncBrandLogoOverrideQuery(
        queryClient,
        { scope, target_key: normalizedTargetKey } as AdminBrandLogoOverrideDto,
        true,
      )
    },
  }
}

export function useAdminBrandLogoResource() {
  return createAdminBrandLogoResource(useQueryClient())
}

async function cancelBrandLogoOverrideQuery(queryClient: BrandLogoOverrideQueryClient) {
  await queryClient.cancelQueries({ queryKey: getListAdminBrandLogoOverridesQueryKey() })
}

function syncBrandLogoOverrideQuery(
  queryClient: BrandLogoOverrideQueryClient,
  override: AdminBrandLogoOverrideDto,
  remove: boolean,
) {
  const queryKey = getListAdminBrandLogoOverridesQueryKey()
  const cached = queryClient.getQueryData<ApiResponseAdminBrandLogoOverrideList>(queryKey)
  const prior = cached?.data?.overrides ?? []
  const withoutTarget = prior.filter(
    (item) => item.scope !== override.scope || item.target_key !== override.target_key,
  )
  const overrides = remove ? withoutTarget : [...withoutTarget, override]
  const next: ApiResponseAdminBrandLogoOverrideList = {
    code: cached?.code ?? 0,
    msg: cached?.msg ?? 'ok',
    data: { overrides },
  }
  queryClient.setQueryData<ApiResponseAdminBrandLogoOverrideList>(queryKey, next)
  hydrateAdminBrandLogoOverrides(overrides)
}
