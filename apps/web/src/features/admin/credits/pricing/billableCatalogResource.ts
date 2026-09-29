import {
  deleteControlCatalogItem,
  listControlBillingCatalog,
  publishControlCatalogPrices,
  setControlCatalogAvailability,
} from '../../../../shared/api/generated-query/control/control-billing/control-billing'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../../shared/api/contractKernel'
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
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await deleteControlCatalogItem(sku, {
      headers: mutationHeaders(options, operatorReason),
    })
    const data = requireContractData<{ sku: string }>(body, '删除已归档功能失败')
    return data.sku
  } catch (error) {
    throw withControlCopy(error, '删除已归档功能失败')
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
      { headers: mutationHeaders(options, operatorReason) },
    )
    return normalizeAvailability(
      requireContractData<BillableItemAvailability>(
        body,
        enabled ? '启用功能失败' : '停用功能失败',
      ),
    )
  } catch (error) {
    throw withControlCopy(error, enabled ? '启用功能失败' : '停用功能失败')
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
      { headers: mutationHeaders(options, operatorReason) },
    )
    const data = requireContractData<{
      prices: Partial<Record<PricingVariant, BillingPrice>>
    }>(body, '发布价格失败')
    return Object.fromEntries(
      Object.entries(data.prices).map(([resolution, price]) => [resolution, normalizePrice(price)]),
    ) as Partial<Record<PricingVariant, AdminBillableItemPrice>>
  } catch (error) {
    throw withControlCopy(error, '发布价格失败')
  }
}

async function fetchAdminBillableItems(): Promise<AdminBillableItemCard[]> {
  try {
    const body = await listControlBillingCatalog()
    const data = requireContractData<{ items: BillableItemCatalogCard[] }>(body, '加载功能定价失败')
    return data.items.map(normalizeBillableItem)
  } catch (error) {
    throw withControlCopy(error, '加载功能定价失败')
  }
}

function mutationHeaders(options: BillableCatalogMutationOptions, operatorReason: string) {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? newRequestId(),
    operatorReason,
  })
}

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw controlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function controlError(code: string, message: string): Error & { code?: string | undefined } {
  const error = new Error(message) as Error & { code?: string | undefined }
  error.code = code
  return error
}

// withControlCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text.
function withControlCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError(code === 409 ? 'idempotency_conflict' : code, '操作已提交，请勿重复操作')
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因')
  }
  if (code === 'control_not_found' || code === 'not_found' || code === 404) {
    return controlError('control_not_found', '功能不存在')
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback)
}

function readErrorCode(error: unknown): string | number | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    if (typeof code === 'string' || typeof code === 'number') return code
  }
  if (error && typeof error === 'object' && 'status' in error) {
    const status = (error as { status?: unknown }).status
    if (status === 404 || status === 409) return status
  }
  return undefined
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
