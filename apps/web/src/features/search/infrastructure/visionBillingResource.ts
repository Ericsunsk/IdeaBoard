import { api } from '../../../shared/api/client'
import { resolveApiData, type ApiResponseWithData } from '../../../shared/api/apiResponse'
import { i18n } from '../../../shared/i18n'

// Vision search pricing boundary (issue #809). The generated `billing`
// client is gone but the backend user plane still serves
// `POST /api/user/billing/quotes` with the shared envelope, so this
// boundary transports through the central `api.request` with a local view
// type.
const VISION_SEARCH_SKU = 'ai-search:vision'

export type VisionBillingQuote = {
  sku: string
  output_resolution: string
  display_name: string
  price_version_id: number
  price_version: number
  unit_credits: number
  quantity: number
  total_credits: number
  quoted_at: string
}

export async function fetchVisionSearchPrice(signal?: AbortSignal): Promise<number> {
  const quote = await resolveApiData(
    api.request<ApiResponseWithData<VisionBillingQuote>>('/user/billing/quotes', {
      method: 'POST',
      body: {
        sku: VISION_SEARCH_SKU,
        output_resolution: 'default',
        quantity: 1,
      } as unknown as Record<string, unknown>,
      ...(signal == null ? {} : { signal }),
    }),
    i18n.global.t('search.priceUnavailable'),
  )
  return quote.total_credits
}
