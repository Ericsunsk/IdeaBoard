import { api } from '../../../../shared/api/client'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../../shared/api/apiResponse'
import { stripUndefined } from '../../../../shared/api/stripUndefined'
import type {
  AdminBillableItemAvailability,
  AdminBillableItemCard,
  AdminBillableItemPrice,
  AdminBillableItemStatus,
  PricingVariant,
} from './billableCatalogTypes'

// Admin billing catalog boundary (issue #809). The generated
// `admin-billing` client is gone but the backend admin plane still serves
// `/api/admin/billing/catalog/*` with the shared envelope, so this boundary
// transports through the central `api.request` with local wire types.
export type BillableItemCatalogCard = {
  sku: string
  namespace: string
  product_kind: string
  display_name: string
  description: string
  sort_order: number
  status: AdminBillableItemStatus
  current_prices?: Partial<Record<PricingVariant, BillingPrice>>
  pricing_variants: PricingVariant[]
  updated_at?: string
}

export type BillingPrice = {
  output_resolution: PricingVariant
  price_version_id: number
  version: number
  unit_credits: number
  created_at: string
}

export type BillableItemAvailability = {
  sku: string
  status: 'active' | 'disabled'
  updated_at: string
}

export interface AdminBillableCatalogResource {
  list: () => Promise<AdminBillableItemCard[]>
  publishPrices: (
    sku: string,
    prices: Partial<Record<PricingVariant, number | undefined>>,
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
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ sku: string }>>(
      `/admin/billing/catalog/${encodeURIComponent(sku)}`,
      { method: 'DELETE' },
    ),
    '删除已归档功能失败',
  )
  return data.sku
}

export async function setBillableItemAvailability(
  sku: string,
  enabled: boolean,
): Promise<AdminBillableItemAvailability> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<BillableItemAvailability>>(
      `/admin/billing/catalog/${encodeURIComponent(sku)}/availability`,
      {
        method: 'PUT',
        body: { enabled } as unknown as Record<string, unknown>,
      },
    ),
    enabled ? '启用功能失败' : '停用功能失败',
  )
  return normalizeAvailability(data)
}

export async function publishBillableItemPrices(
  sku: string,
  prices: Partial<Record<PricingVariant, number | undefined>>,
): Promise<Partial<Record<PricingVariant, AdminBillableItemPrice>>> {
  const requestPrices = stripUndefined(prices) as Record<PricingVariant, number>
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ prices: Partial<Record<PricingVariant, BillingPrice>> }>>(
      `/admin/billing/catalog/${encodeURIComponent(sku)}/prices`,
      {
        method: 'POST',
        body: { prices: requestPrices } as unknown as Record<string, unknown>,
      },
    ),
    '发布价格失败',
  )
  return Object.fromEntries(
    Object.entries(data.prices).map(([resolution, price]) => [resolution, normalizePrice(price)]),
  ) as Partial<Record<PricingVariant, AdminBillableItemPrice>>
}

async function fetchAdminBillableItems(): Promise<AdminBillableItemCard[]> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ items: BillableItemCatalogCard[] }>>(
      '/admin/billing/catalog',
      { method: 'GET' },
    ),
    '加载功能定价失败',
  )
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
