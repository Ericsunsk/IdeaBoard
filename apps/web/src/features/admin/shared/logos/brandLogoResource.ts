import { adminControlUnavailable } from '../lib/controlGuidance'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import {
  replaceAdminBrandLogoOverrides,
  type AdminBrandLogoKey,
  type AdminBrandLogoScope,
} from './brandLogos'

// Admin brand logo override boundary (W2 transports, fail-closed). The
// Control Plane has no brand-logo operations, so the override read and
// save/remove fail closed with guidance instead of calling the deleted
// `/api/admin/brand-logo-overrides*` routes. Local override hydration
// stays (defaults render when nothing is stored). The models.dev logo
// proxy URLs in `brandLogos.ts` are a separate backend gap (images fall
// back to placeholders via onError). See the stream report.
type AdminBrandLogoOverrideDto = {
  scope: AdminBrandLogoScope
  target_key: string
  logo_key: AdminBrandLogoKey
  created_at?: string
  updated_at?: string
}

export type ApiResponseAdminBrandLogoOverrideList = {
  data: {
    overrides: AdminBrandLogoOverrideDto[]
  }
}

export const ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY = ['admin', 'brand-logo-overrides'] as const

type BrandLogoOverrideQueryClient = Pick<QueryClient, 'cancelQueries'>

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
    async save(
      _scope: AdminBrandLogoScope,
      _targetKey: string | number,
      _logoKey: AdminBrandLogoKey,
    ) {
      await cancelBrandLogoOverrideQuery(queryClient)
      throw adminControlUnavailable('logo-upload')
    },

    async remove(_scope: AdminBrandLogoScope, _targetKey: string | number) {
      await cancelBrandLogoOverrideQuery(queryClient)
      throw adminControlUnavailable('logo-upload')
    },
  }
}

export function useAdminBrandLogoResource() {
  return createAdminBrandLogoResource(useQueryClient())
}

async function cancelBrandLogoOverrideQuery(queryClient: BrandLogoOverrideQueryClient) {
  await queryClient.cancelQueries({ queryKey: ADMIN_BRAND_LOGO_OVERRIDES_QUERY_KEY })
}
