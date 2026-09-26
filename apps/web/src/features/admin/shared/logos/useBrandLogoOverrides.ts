import type { MaybeRefOrGetter } from 'vue'
import { watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { adminControlUnavailable } from '../lib/controlGuidance'
import {
  ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY,
  hydrateAdminBrandLogoOverrides,
  type ApiResponseAdminBrandLogoOverrideList,
} from './brandLogoResource'

export function useBrandLogoOverrides(enabled: MaybeRefOrGetter<boolean>) {
  const query = useQuery<ApiResponseAdminBrandLogoOverrideList, Error>({
    queryKey: ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY,
    queryFn: () => Promise.reject(adminControlUnavailable('logo-upload')),
    enabled,
    staleTime: Infinity,
    retry: false,
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
