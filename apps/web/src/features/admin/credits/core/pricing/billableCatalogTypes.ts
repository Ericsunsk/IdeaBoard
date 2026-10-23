import type { OutputResolution as CatalogResolution } from '@/core/contracts/generationContract'

export type AdminBillableItemStatus = 'unpriced' | 'active' | 'disabled' | 'archived'
// Pricing and pool resolutions are the shared catalog values (1k/2k/4k):
// this module aliases the single source instead of declaring a competing
// enum. Never add a "default" variant here.
export type PricingVariant = CatalogResolution

export interface AdminBillableItemPrice {
  priceVersionId: number
  version: number
  unitCredits: number
  createdAt: string
}

export interface AdminBillableItemAvailability {
  sku: string
  status: Extract<AdminBillableItemStatus, 'active' | 'disabled'>
  updatedAt: string
}

export interface AdminBillableItemCard {
  sku: string
  namespace: string
  productKind: string
  displayName: string
  sortOrder: number
  status: AdminBillableItemStatus
  pricingVariants: PricingVariant[]
  currentPrices: Partial<Record<PricingVariant, AdminBillableItemPrice>>
  updatedAt?: string | undefined
}

// Service Model Pool shapes (#849, ADR-0012): one Service owns an
// unordered set of {model, enabled, supported_resolutions} entries plus
// a monotonic pool version pinned into routing snapshots. Entries carry
// no priority, fallback, or weight — never add those fields here.
// Resolutions alias the shared catalog values, never a local copy.
export type PoolResolution = CatalogResolution

export interface AdminModelPoolEntry {
  modelId: string
  enabled: boolean
  resolutions: PoolResolution[]
}

export interface AdminModelPool {
  service: string
  sku: string
  version: number
  entries: AdminModelPoolEntry[]
}

export interface AdminModelPoolSaveResult extends AdminModelPool {
  changed: boolean
}

export interface AdminPoolCatalogModel {
  id: string
  name: string
}
