import {
  deleteAdminArchivedBillableItem,
  listAdminBillableItems,
  publishAdminBillingPrice,
  setAdminBillableItemAvailability,
} from '../../../../shared/api/generated-query/admin-billing/admin-billing'
import type {
  BillableItemCatalogCard,
  BillableItemAvailability,
  BillingPrice,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'
import type {
  AdminBillableItemAvailability,
  AdminBillableItemCard,
  AdminBillableItemPrice,
  PricingVariant,
} from './billableCatalogTypes'

export interface AdminBillableCatalogResource {
  list: () => Promise<AdminBillableItemCard[]>
  publishPrices: (
    sku: string,
    prices: Record<PricingVariant, number | undefined>,
  ) => Promise<Partial<Record<PricingVariant, AdminBillableItemPrice>>>
  setAvailability: (sku: string, enabled: boolean) => Promise<AdminBillableItemAvailability>
  deleteArchived: (sku: string) => Promise<string>
}

export function createAdminBillableCatalogResource(): AdminBillableCatalogResource {
  return {
    list: fetchAdminBillableItems,
    publishPrices: publishBillableItemPrices,
    setAvailability: setBillableItemAvailability,
    deleteArchived: deleteArchivedBillableItem,
  }
}

export async function deleteArchivedBillableItem(sku: string): Promise<string> {
  const data = await resolveApiData(deleteAdminArchivedBillableItem(sku), '删除已归档功能失败')
  return data.sku
}

export async function setBillableItemAvailability(
  sku: string,
  enabled: boolean,
): Promise<AdminBillableItemAvailability> {
  const data = await resolveApiData(
    setAdminBillableItemAvailability(sku, { enabled }),
    enabled ? '启用功能失败' : '停用功能失败',
  )
  return normalizeAvailability(data)
}

export async function publishBillableItemPrices(
  sku: string,
  prices: Record<PricingVariant, number | undefined>,
): Promise<Partial<Record<PricingVariant, AdminBillableItemPrice>>> {
  const requestPrices = Object.fromEntries(
    Object.entries(prices).filter(([, price]) => price !== undefined),
  ) as Record<PricingVariant, number>
  const data = await resolveApiData(
    publishAdminBillingPrice(sku, { prices: requestPrices }),
    '发布价格失败',
  )
  return Object.fromEntries(
    Object.entries(data.prices).map(([resolution, price]) => [resolution, normalizePrice(price)]),
  ) as Partial<Record<PricingVariant, AdminBillableItemPrice>>
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
    sortOrder: item.sort_order,
    status: item.status,
    pricingVariants: normalizePricingVariants(item.pricing_variants),
    currentPrices: Object.fromEntries(
      Object.entries(item.current_prices ?? {}).map(([resolution, price]) => [
        resolution,
        normalizePrice(price),
      ]),
    ) as Partial<Record<PricingVariant, AdminBillableItemPrice>>,
    updatedAt: item.updated_at,
  }
}

function normalizePricingVariants(raw: unknown): PricingVariant[] {
  if (!Array.isArray(raw)) return ['1k', '2k', '4k']
  const variants = raw.filter(
    (value): value is PricingVariant =>
      value === 'default' || value === '1k' || value === '2k' || value === '4k',
  )
  return variants.length > 0 ? variants : ['1k', '2k', '4k']
}

function normalizePrice(price: BillingPrice): AdminBillableItemPrice {
  return {
    priceVersionId: price.price_version_id,
    version: price.version,
    unitCredits: price.unit_credits,
    createdAt: price.created_at,
  }
}

function normalizeAvailability(
  availability: BillableItemAvailability,
): AdminBillableItemAvailability {
  return {
    sku: availability.sku,
    status: availability.status,
    updatedAt: availability.updated_at,
  }
}
