import { createBillingQuote } from '../../../shared/api/generated-query/billing/billing'
import { resolveApiData } from '../../../shared/api/apiResponse'
import { i18n } from '../../../shared/i18n'

const VISION_SEARCH_SKU = 'ai-search:vision'

export async function fetchVisionSearchPrice(signal?: AbortSignal): Promise<number> {
  const quote = await resolveApiData(
    createBillingQuote(
      {
        sku: VISION_SEARCH_SKU,
        output_resolution: 'default',
        quantity: 1,
      },
      { signal },
    ),
    i18n.global.t('search.priceUnavailable'),
  )
  return quote.total_credits
}
