import { api } from '../../../../shared/api/client'
import {
  resolveApiData,
  resolveApiOk,
  type ApiResponseWithData,
} from '../../../../shared/api/apiResponse'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import {
  replaceAdminBrandLogoOverrides,
  type AdminBrandLogoKey,
  type AdminBrandLogoScope,
} from './brandLogos'

// Admin brand logo override boundary (issue #809). The generated
// `admin-brand-logos` client is gone but the backend admin plane still
// serves `/api/admin/brand-logo-overrides*` with the shared envelope, so
// this boundary transports through the central `api.request`.
export type AdminBrandLogoOverrideDto = {
  scope: AdminBrandLogoScope
  target_key: string
  logo_key: AdminBrandLogoKey
  created_at?: string
  updated_at?: string
}

export type ApiResponseAdminBrandLogoOverrideList = ApiResponseWithData<{
  overrides: AdminBrandLogoOverrideDto[]
}>

export const ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY = ['admin', 'brand-logo-overrides'] as const

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
        api.request<ApiResponseWithData<AdminBrandLogoOverrideDto>>(
          `/admin/brand-logo-overrides/${encodeURIComponent(scope)}/${encodeURIComponent(String(targetKey))}`,
          {
            method: 'PUT',
            body: { logo_key: logoKey } as unknown as Record<string, unknown>,
          },
        ),
        '保存品牌 Logo 配置失败',
      )
      syncBrandLogoOverrideQuery(queryClient, data, false)
    },

    async remove(scope: AdminBrandLogoScope, targetKey: string | number) {
      const normalizedTargetKey = String(targetKey)
      await cancelBrandLogoOverrideQuery(queryClient)
      await resolveApiOk(
        api.request<{ code: number; msg: string }>(
          `/admin/brand-logo-overrides/${encodeURIComponent(scope)}/${encodeURIComponent(normalizedTargetKey)}`,
          { method: 'DELETE' },
        ),
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
  await queryClient.cancelQueries({ queryKey: ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY })
}

function syncBrandLogoOverrideQuery(
  queryClient: BrandLogoOverrideQueryClient,
  override: AdminBrandLogoOverrideDto,
  remove: boolean,
) {
  const queryKey = ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY
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
