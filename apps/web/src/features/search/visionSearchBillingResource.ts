import { createBillingQuote } from '../../shared/api/generated-query/billing/billing'
import { resolveApiData } from '../../shared/api/apiResponse'

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
    '视觉搜索价格暂不可用',
  )
  return quote.total_credits
}
