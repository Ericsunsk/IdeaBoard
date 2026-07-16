export type AdminBillableItemStatus = 'unpriced' | 'active' | 'disabled' | 'archived'

export interface AdminBillableItemPrice {
  priceVersionId: number
  version: number
  unitCredits: number
  createdBy: number
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
  description: string
  sortOrder: number
  status: AdminBillableItemStatus
  currentPrice?: AdminBillableItemPrice
  updatedAt?: string
}
