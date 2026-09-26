import { createWorkspaceBillingQuote } from '../../../shared/api/generated-query/public/workspace-billing/workspace-billing'
import { i18n } from '../../../shared/i18n'

// Vision search pricing boundary (issue #809).
//
// Reads the server-side quote for the `ai-search:vision` SKU through the
// generated `public/workspace-billing` hook (`POST
// /api/v1/workspaces/{id}/billing-quotes`, no persistence, no price
// fields). The deleted `POST /api/user/billing/quotes` route is gone.
// Price feedback stays best-effort (the paid submit path remains
// authoritative): callers treat any throw as unavailable.
const VISION_SEARCH_SKU = 'ai-search:vision'

export async function fetchVisionSearchPrice(
  workspaceId: string,
  signal?: AbortSignal,
): Promise<number> {
  const id = workspaceId.trim()
  if (!id) throw new Error(i18n.global.t('search.priceUnavailable'))
  const envelope = await createWorkspaceBillingQuote(
    id,
    { data: { sku: VISION_SEARCH_SKU } },
    { ...(signal == null ? {} : { signal }) },
  )
  const total = (envelope?.data as { total_credits?: unknown } | undefined)?.total_credits
  if (typeof total !== 'number' || !Number.isFinite(total)) {
    throw new Error(i18n.global.t('search.priceUnavailable'))
  }
  return total
}
