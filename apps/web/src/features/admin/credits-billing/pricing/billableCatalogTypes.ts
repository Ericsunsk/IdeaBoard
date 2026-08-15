export type AdminBillableItemStatus = 'unpriced' | 'active' | 'disabled' | 'archived'
export type OutputResolution = '1k' | '2k' | '4k'

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
  currentPrices: Partial<Record<OutputResolution, AdminBillableItemPrice>>
  updatedAt?: string
}
