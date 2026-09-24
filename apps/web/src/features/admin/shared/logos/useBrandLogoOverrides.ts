import type { MaybeRefOrGetter } from 'vue'
import { watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api } from '../../../../shared/api/client'
import {
  ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY,
  hydrateAdminBrandLogoOverrides,
  type ApiResponseAdminBrandLogoOverrideList,
} from './brandLogoResource'

export function useBrandLogoOverrides(enabled: MaybeRefOrGetter<boolean>) {
  const query = useQuery({
    queryKey: ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY,
    queryFn: ({ signal }) =>
      api.request<ApiResponseAdminBrandLogoOverrideList>('/admin/brand-logo-overrides', {
        method: 'GET',
        ...(signal == null ? {} : { signal }),
      }),
    enabled,
    staleTime: Infinity,
  })

  watch(
    () => query.data.value?.data?.overrides,
    (overrides) => {
      if (overrides) hydrateAdminBrandLogoOverrides(overrides)
    },
    { immediate: true },
  )

  return query
}
