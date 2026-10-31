// Site branding transport (API boundary: core must not import generated DTOs).
//
// Thin `api/*` wrapper owning the public branding transport plus the
// transport-DTO -> domain translation. `core/composables/useSiteBranding`
// keeps Vue wiring and the pure normalize/validate helpers only.
import {
  getGetPublicBrandingSettingQueryKey,
  getPublicBrandingSetting,
} from './generated-query/public/user/user'
import type {
  PublicBrandingSetting,
  PublicBrandingSettingEnvelope,
} from './generated-query/public/model'
import {
  createContractDataSelector,
  resolveContractData,
  type ContractSuccessEnvelope,
} from './http/contractKernel'

export type BrandingSetting = {
  project_name: string
  logo_url: string
}

export type ApiResponseBrandingSetting = ContractSuccessEnvelope<BrandingSetting>

export const SITE_BRANDING_QUERY_KEY = getGetPublicBrandingSettingQueryKey()

function toBrandingSetting(dto: PublicBrandingSetting): BrandingSetting {
  return {
    project_name: dto.project_name,
    logo_url: dto.logo_url,
  }
}

export function createSiteBrandingSelector(fallback: string) {
  const selectEnvelope = createContractDataSelector<PublicBrandingSetting>(fallback)
  return (envelope: unknown): BrandingSetting => toBrandingSetting(selectEnvelope(envelope))
}

export function fetchSiteBrandingEnvelope(
  signal?: AbortSignal,
): Promise<PublicBrandingSettingEnvelope> {
  return getPublicBrandingSetting({
    ...(signal == null ? {} : { signal }),
    authFailure: 'silent',
  })
}

export async function fetchSiteBrandingSetting(
  fallback: string,
  signal?: AbortSignal,
): Promise<BrandingSetting> {
  return toBrandingSetting(
    await resolveContractData<PublicBrandingSetting>(fetchSiteBrandingEnvelope(signal), fallback),
  )
}
