import { listAdminBillableItems } from '../../../../shared/api/generated-query/admin-billing/admin-billing'
import type { BillableItemCatalogCard } from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'
import type { AdminBillableItemCard } from './billableCatalogTypes'

export interface AdminBillableCatalogResource {
  list: () => Promise<AdminBillableItemCard[]>
}

export function createAdminBillableCatalogResource(): AdminBillableCatalogResource {
  return { list: fetchAdminBillableItems }
}

export async function fetchAdminBillableItems(): Promise<AdminBillableItemCard[]> {
  const data = await resolveApiData(listAdminBillableItems(), '加载功能定价失败')
  return data.items.map(normalizeBillableItem)
}

function normalizeBillableItem(item: BillableItemCatalogCard): AdminBillableItemCard {
  return {
    sku: item.sku,
    namespace: item.namespace,
    productKind: item.product_kind,
    displayName: item.display_name,
    description: item.description,
    sortOrder: item.sort_order,
    status: item.status,
    currentPrice: item.current_price
      ? {
          priceVersionId: item.current_price.price_version_id,
          version: item.current_price.version,
          unitCredits: item.current_price.unit_credits,
          enabled: item.current_price.enabled,
          updatedAt: item.current_price.updated_at,
        }
      : undefined,
    updatedAt: item.updated_at,
  }
}
