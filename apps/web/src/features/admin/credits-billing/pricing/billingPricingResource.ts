import {
  createAdminBillingPrice,
  createAdminBillingSKU,
  getAdminBillingCoverage as listAdminBillingCoverage,
  listAdminBillingPrices,
  listAdminBillingSKUs,
  setAdminBillingPriceEnabled,
  updateAdminBillingSKU,
} from '../../../../shared/api/generated-query/admin-billing/admin-billing'
import type {
  BillingCoverageItem as GeneratedBillingCoverageItem,
  BillingCatalogItem as GeneratedBillingCatalogItem,
  BillingPrice as GeneratedBillingPrice,
} from '../../../../shared/api/generated-query/model'
import { requireApiData } from '../../shared/queries/adminApiResponse'
import type {
  AdminBillingCatalogItem,
  AdminBillingCoverageItem,
  AdminBillingCatalogPage,
  AdminBillingPrice,
  AdminBillingPricePage,
  CreateAdminBillingPriceInput,
  CreateAdminBillingSKUInput,
  UpdateAdminBillingSKUInput,
} from './billingPricingTypes'

export interface AdminBillingPricingResource {
  listCoverage: () => Promise<AdminBillingCoverageItem[]>
  listSKUs: (input?: { afterSku?: string; limit?: number }) => Promise<AdminBillingCatalogPage>
  createSKU: (input: CreateAdminBillingSKUInput) => Promise<AdminBillingCatalogItem>
  updateSKU: (sku: string, input: UpdateAdminBillingSKUInput) => Promise<AdminBillingCatalogItem>
  listPrices: (
    sku: string,
    input?: { beforeVersion?: number; limit?: number },
  ) => Promise<AdminBillingPricePage>
  createPrice: (sku: string, input: CreateAdminBillingPriceInput) => Promise<AdminBillingPrice>
  setPriceEnabled: (
    sku: string,
    version: number,
    input: { enabled: boolean; reason: string },
  ) => Promise<AdminBillingPrice>
}

export function createAdminBillingPricingResource(): AdminBillingPricingResource {
  return {
    listCoverage: fetchAdminBillingCoverage,
    listSKUs: fetchAdminBillingSKUs,
    createSKU: createAdminBillingSKURecord,
    updateSKU: updateAdminBillingSKURecord,
    listPrices: fetchAdminBillingPrices,
    createPrice: createAdminBillingPriceRecord,
    setPriceEnabled: setAdminBillingPriceState,
  }
}

export async function fetchAdminBillingCoverage(): Promise<AdminBillingCoverageItem[]> {
  const data = requireApiData(await listAdminBillingCoverage(), '加载 AI Workflow 计费覆盖失败')
  return data.items.map(normalizeCoverageItem)
}

export async function fetchAdminBillingSKUs(
  input: { afterSku?: string; limit?: number } = {},
): Promise<AdminBillingCatalogPage> {
  const data = requireApiData(
    await listAdminBillingSKUs({ after_sku: input.afterSku, limit: input.limit }),
    '加载 SKU Catalog 失败',
  )
  return {
    items: data.items.map(normalizeCatalogItem),
    nextAfterSku: data.next_after_sku,
  }
}

export async function createAdminBillingSKURecord(
  input: CreateAdminBillingSKUInput,
): Promise<AdminBillingCatalogItem> {
  const data = requireApiData(
    await createAdminBillingSKU({
      sku: input.sku.trim().toLocaleLowerCase(),
      display_name: input.displayName.trim(),
      description: input.description.trim(),
      enabled: input.enabled,
      reason: input.reason.trim(),
    }),
    '创建 SKU 失败',
  )
  return normalizeCatalogItem(data)
}

export async function updateAdminBillingSKURecord(
  sku: string,
  input: UpdateAdminBillingSKUInput,
): Promise<AdminBillingCatalogItem> {
  const data = requireApiData(
    await updateAdminBillingSKU(sku, {
      display_name: input.displayName.trim(),
      description: input.description.trim(),
      enabled: input.enabled,
      reason: input.reason.trim(),
    }),
    '更新 SKU 失败',
  )
  return normalizeCatalogItem(data)
}

export async function fetchAdminBillingPrices(
  sku: string,
  input: { beforeVersion?: number; limit?: number } = {},
): Promise<AdminBillingPricePage> {
  const data = requireApiData(
    await listAdminBillingPrices(sku, {
      before_version: input.beforeVersion,
      limit: input.limit,
    }),
    '加载价格版本失败',
  )
  return {
    prices: data.prices.map(normalizePrice),
    nextBeforeVersion: data.next_before_version,
  }
}

export async function createAdminBillingPriceRecord(
  sku: string,
  input: CreateAdminBillingPriceInput,
): Promise<AdminBillingPrice> {
  const data = requireApiData(
    await createAdminBillingPrice(sku, {
      unit_credits: input.unitCredits,
      enabled: input.enabled,
      reason: input.reason.trim(),
    }),
    '发布价格版本失败',
  )
  return normalizePrice(data)
}

export async function setAdminBillingPriceState(
  sku: string,
  version: number,
  input: { enabled: boolean; reason: string },
): Promise<AdminBillingPrice> {
  const data = requireApiData(
    await setAdminBillingPriceEnabled(sku, version, {
      enabled: input.enabled,
      reason: input.reason.trim(),
    }),
    input.enabled ? '启用价格版本失败' : '停用价格版本失败',
  )
  return normalizePrice(data)
}

function normalizeCatalogItem(item: GeneratedBillingCatalogItem): AdminBillingCatalogItem {
  return {
    sku: item.sku,
    displayName: item.display_name,
    description: item.description,
    enabled: item.enabled,
    currentPriceVersionId: item.current_price_version_id,
    currentPrice: item.current_price ? normalizePrice(item.current_price) : undefined,
    createdAt: item.created_at,
    updatedAt: item.updated_at,
  }
}

function normalizeCoverageItem(item: GeneratedBillingCoverageItem): AdminBillingCoverageItem {
  return {
    feature: item.feature,
    model: item.model,
    sku: item.sku,
    skuConfigured: item.sku_configured,
    skuEnabled: item.sku_enabled,
    priceConfigured: item.price_configured,
    priceEnabled: item.price_enabled,
    unitCredits: item.unit_credits,
  }
}

function normalizePrice(price: GeneratedBillingPrice): AdminBillingPrice {
  return {
    priceVersionId: price.price_version_id,
    version: price.version,
    unitCredits: price.unit_credits,
    enabled: price.enabled,
    createdBy: price.created_by,
    createdAt: price.created_at,
    updatedBy: price.updated_by,
    updatedAt: price.updated_at,
  }
}
