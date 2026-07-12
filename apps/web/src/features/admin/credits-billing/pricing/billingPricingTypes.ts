import type { WorkflowFeature } from '../../../../shared/domain/workflowFeatures'

export interface AdminBillingCoverageItem {
  feature: WorkflowFeature
  model: string
  sku: string
  skuConfigured: boolean
  skuEnabled: boolean
  priceConfigured: boolean
  priceEnabled: boolean
  unitCredits?: number
}

export interface AdminBillingPrice {
  priceVersionId: number
  version: number
  unitCredits: number
  enabled: boolean
  createdBy: number
  createdAt: string
  updatedBy: number
  updatedAt: string
}

export interface AdminBillingCatalogItem {
  sku: string
  displayName: string
  description: string
  enabled: boolean
  currentPriceVersionId?: number
  currentPrice?: AdminBillingPrice
  createdAt: string
  updatedAt: string
}

export interface AdminBillingCatalogPage {
  items: AdminBillingCatalogItem[]
  nextAfterSku?: string
}

export interface AdminBillingPricePage {
  prices: AdminBillingPrice[]
  nextBeforeVersion?: number
}

export interface CreateAdminBillingSKUInput {
  sku: string
  displayName: string
  description: string
  enabled: boolean
  reason: string
}

export interface UpdateAdminBillingSKUInput {
  displayName: string
  description: string
  enabled: boolean
  reason: string
}

export interface CreateAdminBillingPriceInput {
  unitCredits: number
  enabled: boolean
  reason: string
}

export type BillingCatalogFilter = 'all' | 'ai-image' | 'unpriced' | 'disabled'

export interface BillingSKUForm {
  sku: string
  displayName: string
  description: string
  enabled: boolean
  reason: string
}

export interface BillingPriceForm {
  unitCredits: number | null
  enabled: boolean
  reason: string
}

export interface BillingSKUFormErrors {
  sku?: string
  displayName?: string
  description?: string
  reason?: string
}

export interface BillingPriceFormErrors {
  unitCredits?: string
  reason?: string
}
