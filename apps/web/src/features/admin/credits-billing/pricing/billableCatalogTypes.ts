export type AdminBillableItemStatus = 'unpriced' | 'active' | 'disabled' | 'archived'
export type PricingVariant = 'default' | '1k' | '2k' | '4k'
export type OutputResolution = Exclude<PricingVariant, 'default'>

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
