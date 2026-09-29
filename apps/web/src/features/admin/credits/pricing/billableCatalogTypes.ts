export type AdminBillableItemStatus = 'unpriced' | 'active' | 'disabled' | 'archived'
export type PricingVariant = '1k' | '2k' | '4k'
export type OutputResolution = PricingVariant

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
export type PoolResolution = '1k' | '2k' | '4k'

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
