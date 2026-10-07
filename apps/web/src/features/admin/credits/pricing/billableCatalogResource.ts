import {
  deleteControlCatalogItem,
  listControlBillingCatalog,
  publishControlCatalogPrices,
  setControlCatalogAvailability,
} from '../../../../shared/api/generated-query/control/control-billing/control-billing'
import { requireContractData } from '../../../../shared/api/contractKernel'
import { stripUndefined } from '../../../../shared/api/stripUndefined'
import {
  OUTPUT_RESOLUTION_OPTIONS,
  parseOutputResolution,
} from '../../../../shared/domain/generationContract'
import type {
  AdminBillableItemAvailability,
  AdminBillableItemCard,
  AdminBillableItemPrice,
  AdminBillableItemStatus,
  PricingVariant,
} from './billableCatalogTypes'
import {
  buildControlMutationHeaders,
  requireOperatorReason,
} from '../../shared/lib/controlMutation'
import { withControlCopy } from '../../shared/lib/controlCopy'

const BILLABLE_ITEM_CONTROL_MESSAGES = {
  control_not_found: '功能不存在',
  not_found: '功能不存在',
  404: '功能不存在',
} as const

// Admin billing catalog boundary (W2 transports). Reads and writes go
// through the Control Plane billing operations with the contract envelope
// (`{data}` over the same catalog service projections); every mutation
// carries a header-only Idempotency-Key plus an operator reason (mirrored
// into the body `reason` field where the schema accepts it) and emits a
// control audit record. The deleted `/api/admin/billing/catalog/*` routes
// are gone; this module is the single transport.
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

export type BillableCatalogMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

export interface AdminBillableCatalogResource {
  list: () => Promise<AdminBillableItemCard[]>
  publishPrices: (
    sku: string,
    prices: Partial<Record<PricingVariant, number | undefined>>,
    options: BillableCatalogMutationOptions,
  ) => Promise<Partial<Record<PricingVariant, AdminBillableItemPrice>>>
  setAvailability: (
    sku: string,
    enabled: boolean,
    options: BillableCatalogMutationOptions,
  ) => Promise<AdminBillableItemAvailability>
  deleteArchived: (sku: string, options: BillableCatalogMutationOptions) => Promise<string>
}

export function createAdminBillableCatalogResource(): AdminBillableCatalogResource {
  return {
    list: fetchAdminBillableItems,
    publishPrices: publishBillableItemPrices,
    setAvailability: setBillableItemAvailability,
    deleteArchived: deleteArchivedBillableItem,
  }
}

export async function deleteArchivedBillableItem(
  sku: string,
  options: BillableCatalogMutationOptions,
): Promise<string> {
  try {
    const body = await deleteControlCatalogItem(sku, {
      headers: buildControlMutationHeaders(options),
    })
    const data = requireContractData<{ sku: string }>(body, '删除已归档功能失败')
    return data.sku
  } catch (error) {
    throw withControlCopy(error, '删除已归档功能失败', BILLABLE_ITEM_CONTROL_MESSAGES)
  }
}

export async function setBillableItemAvailability(
  sku: string,
  enabled: boolean,
  options: BillableCatalogMutationOptions,
): Promise<AdminBillableItemAvailability> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await setControlCatalogAvailability(
      sku,
      { enabled, reason: operatorReason },
      { headers: buildControlMutationHeaders(options) },
    )
    return normalizeAvailability(
      requireContractData<BillableItemAvailability>(
        body,
        enabled ? '启用功能失败' : '停用功能失败',
      ),
    )
  } catch (error) {
    throw withControlCopy(
      error,
      enabled ? '启用功能失败' : '停用功能失败',
      BILLABLE_ITEM_CONTROL_MESSAGES,
    )
  }
}

export async function publishBillableItemPrices(
  sku: string,
  prices: Partial<Record<PricingVariant, number | undefined>>,
  options: BillableCatalogMutationOptions,
): Promise<Partial<Record<PricingVariant, AdminBillableItemPrice>>> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  const requestPrices = stripUndefined(prices) as Record<PricingVariant, number>
  try {
    const body = await publishControlCatalogPrices(
      sku,
      { prices: requestPrices, reason: operatorReason },
      { headers: buildControlMutationHeaders(options) },
    )
    const data = requireContractData<{
      prices: Partial<Record<PricingVariant, BillingPrice>>
    }>(body, '发布价格失败')
    return Object.fromEntries(
      Object.entries(data.prices).map(([resolution, price]) => [resolution, normalizePrice(price)]),
    ) as Partial<Record<PricingVariant, AdminBillableItemPrice>>
  } catch (error) {
    throw withControlCopy(error, '发布价格失败', BILLABLE_ITEM_CONTROL_MESSAGES)
  }
}

async function fetchAdminBillableItems(): Promise<AdminBillableItemCard[]> {
  try {
    const body = await listControlBillingCatalog()
    const data = requireContractData<{ items: BillableItemCatalogCard[] }>(body, '加载功能定价失败')
    return data.items.map(normalizeBillableItem)
  } catch (error) {
    throw withControlCopy(error, '加载功能定价失败', BILLABLE_ITEM_CONTROL_MESSAGES)
  }
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
  if (!Array.isArray(raw)) return [...OUTPUT_RESOLUTION_OPTIONS]
  const variants = raw.filter(
    (value): value is PricingVariant => parseOutputResolution(value) !== null,
  )
  return variants.length > 0 ? variants : [...OUTPUT_RESOLUTION_OPTIONS]
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
