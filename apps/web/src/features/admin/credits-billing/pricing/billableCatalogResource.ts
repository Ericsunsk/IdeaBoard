import {
  listAdminBillableItems,
  publishAdminBillingPrice,
} from '../../../../shared/api/generated-query/admin-billing/admin-billing'
import type {
  BillableItemCatalogCard,
  BillingPrice,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'
import type { AdminBillableItemCard, AdminBillableItemPrice } from './billableCatalogTypes'

export interface AdminBillableCatalogResource {
  list: () => Promise<AdminBillableItemCard[]>
  publishPrice: (sku: string, unitCredits: number) => Promise<AdminBillableItemPrice>
}

export function createAdminBillableCatalogResource(): AdminBillableCatalogResource {
  return { list: fetchAdminBillableItems, publishPrice: publishBillableItemPrice }
}

export async function publishBillableItemPrice(
  sku: string,
  unitCredits: number,
): Promise<AdminBillableItemPrice> {
  const data = await resolveApiData(
    publishAdminBillingPrice(sku, { unit_credits: unitCredits }),
    '发布价格失败',
  )
  return normalizePrice(data)
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
          createdBy: item.current_price.created_by,
          createdAt: item.current_price.created_at,
        }
      : undefined,
    updatedAt: item.updated_at,
  }
}

function normalizePrice(price: BillingPrice): AdminBillableItemPrice {
  return {
    priceVersionId: price.price_version_id,
    version: price.version,
    unitCredits: price.unit_credits,
    createdBy: price.created_by,
    createdAt: price.created_at,
  }
}
