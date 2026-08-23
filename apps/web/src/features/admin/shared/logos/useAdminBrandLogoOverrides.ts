import type { MaybeRefOrGetter } from 'vue'
import { watch } from 'vue'
import { useListAdminBrandLogoOverrides } from '../../../../shared/api/generated-query/admin-brand-logos/admin-brand-logos'
import { hydrateAdminBrandLogoOverrides } from './adminBrandLogoResource'

export function useAdminBrandLogoOverrides(enabled: MaybeRefOrGetter<boolean>) {
  const query = useListAdminBrandLogoOverrides({
    query: { enabled, staleTime: Infinity },
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
